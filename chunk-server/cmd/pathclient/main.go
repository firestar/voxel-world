package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"chunkserver/internal/network"
)

func main() {
	server := flag.String("server", "127.0.0.1:19000", "chunk server UDP address")
	fromX := flag.Int("fromx", 0, "start block X")
	fromY := flag.Int("fromy", 0, "start block Y")
	fromZ := flag.Int("fromz", 0, "start block Z")
	toX := flag.Int("tox", 0, "end block X")
	toY := flag.Int("toy", 0, "end block Y")
	toZ := flag.Int("toz", 0, "end block Z")
	mode := flag.String("mode", "ground", "traversal mode (ground|flying|underground)")
	clearance := flag.Int("clearance", 0, "required vertical clearance in blocks (0 uses server default)")
	maxClimb := flag.Int("maxclimb", 0, "maximum upward climb per step (0 uses server default)")
	maxDrop := flag.Int("maxdrop", 0, "maximum downward drop per step (0 uses server default)")
	flag.Parse()

	req := network.PathRequest{
		EntityID:  "client-test",
		FromX:     *fromX,
		FromY:     *fromY,
		FromZ:     *fromZ,
		ToX:       *toX,
		ToY:       *toY,
		ToZ:       *toZ,
		Mode:      *mode,
		Clearance: *clearance,
		MaxClimb:  *maxClimb,
		MaxDrop:   *maxDrop,
	}
	payload, _ := json.Marshal(req)
	env := network.Envelope{
		Type:      network.MessagePathRequest,
		Timestamp: time.Now().UTC(),
		Seq:       1,
		Payload:   payload,
	}
	data, err := network.Encode(env)
	if err != nil {
		log.Fatalf("encode: %v", err)
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		log.Fatalf("listen udp: %v", err)
	}
	defer conn.Close()

	target, err := net.ResolveUDPAddr("udp", *server)
	if err != nil {
		log.Fatalf("resolve server: %v", err)
	}

	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.WriteToUDP(data, target); err != nil {
		log.Fatalf("send: %v", err)
	}

	buf := make([]byte, 65536)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		log.Fatalf("recv: %v", err)
	}
	envResp, err := network.Decode(buf[:n])
	if err != nil {
		log.Fatalf("decode env: %v", err)
	}
	if envResp.Type != network.MessagePathResponse {
		log.Fatalf("unexpected response type: %s", envResp.Type)
	}
	var resp network.PathResponse
	if err := json.Unmarshal(envResp.Payload, &resp); err != nil {
		log.Fatalf("decode payload: %v", err)
	}
	fmt.Printf("Route for %s (blocks):\n", resp.EntityID)
	for i, step := range resp.Route {
		fmt.Printf(" %d: (%d,%d,%d)\n", i, step.X, step.Y, step.Z)
	}
}
