package network

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Handler func(ctx context.Context, addr *net.UDPAddr, env Envelope)

type Server struct {
	conn    *net.UDPConn
	logger  *log.Logger
	maxSize int
	seq     atomic.Uint64

	mu       sync.RWMutex
	handlers map[MessageType][]Handler
}

func Listen(listenAddr string, logger *log.Logger, maxSize int) (*Server, error) {
	if maxSize <= 0 {
		maxSize = 64 * 1024
	}
	addr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve udp addr: %w", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen udp: %w", err)
	}
	if logger == nil {
		logger = log.New(log.Writer(), "network", log.LstdFlags|log.Lmicroseconds)
	}
	return &Server{
		conn:     conn,
		logger:   logger,
		maxSize:  maxSize,
		handlers: make(map[MessageType][]Handler),
	}, nil
}

func (s *Server) Close() error {
	return s.conn.Close()
}

func (s *Server) Register(msgType MessageType, handler Handler) {
	s.mu.Lock()
	s.handlers[msgType] = append(s.handlers[msgType], handler)
	s.mu.Unlock()
}

func (s *Server) Serve(ctx context.Context) error {
	buffer := make([]byte, s.maxSize)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		s.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, addr, err := s.conn.ReadFromUDP(buffer)
		if err != nil {
			if nErr, ok := err.(net.Error); ok && nErr.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		payload := make([]byte, n)
		copy(payload, buffer[:n])

		env, err := Decode(payload)
		if err != nil {
			s.logger.Printf("decode message from %s: %v", addr, err)
			continue
		}

		handlers := s.handlersFor(env.Type)
		if len(handlers) == 0 {
			continue
		}

		for _, handler := range handlers {
			h := handler
			go h(ctx, addr, env)
		}
	}
}

func (s *Server) handlersFor(msgType MessageType) []Handler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Handler(nil), s.handlers[msgType]...)
}

func (s *Server) Send(addr string, msg MessageType, payload any) error {
	target, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	data, err := s.prepare(msg, payload)
	if err != nil {
		return err
	}
	_, err = s.conn.WriteToUDP(data, target)
	return err
}

func (s *Server) prepare(msgType MessageType, payload any) ([]byte, error) {
	raw, err := encodePayload(payload)
	if err != nil {
		return nil, err
	}
	env := Envelope{
		Type:      msgType,
		Timestamp: time.Now().UTC(),
		Seq:       s.seq.Add(1),
		Payload:   raw,
	}
	return Encode(env)
}

func encodePayload(payload any) ([]byte, error) {
	switch p := payload.(type) {
	case nil:
		return []byte("null"), nil
	case []byte:
		return p, nil
	default:
		return jsonMarshal(payload)
	}
}

func jsonMarshal(v any) ([]byte, error) {
	type marshaler interface {
		MarshalJSON() ([]byte, error)
	}
	if m, ok := v.(marshaler); ok {
		return m.MarshalJSON()
	}
	return json.Marshal(v)
}
