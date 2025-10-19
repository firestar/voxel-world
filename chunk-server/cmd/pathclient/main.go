package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"chunkserver/internal/network"
)

func main() {
	server := flag.String("server", "127.0.0.1:19000", "chunk server UDP address")
	fromX := flag.Int("fromx", 0, "start block X")
	fromY := flag.Int("fromy", 0, "start block Y")
	toX := flag.Int("tox", 0, "end block X")
	toY := flag.Int("toy", 0, "end block Y")
	chunkW := flag.Int("chunkw", 512, "chunk width in blocks")
	chunkD := flag.Int("chunkd", 512, "chunk depth in blocks")
	flag.Parse()

	fromChunkX := floorDiv(*fromX, *chunkW)
	fromChunkY := floorDiv(*fromY, *chunkD)
	toChunkX := floorDiv(*toX, *chunkW)
	toChunkY := floorDiv(*toY, *chunkD)

	req := network.PathRequest{
		EntityID: "client-test",
		FromX:    fromChunkX,
		FromY:    fromChunkY,
		ToX:      toChunkX,
		ToY:      toChunkY,
		Mode:     "ground",
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
	fmt.Printf("Route for %s (chunks):\n", resp.EntityID)
	for i, step := range resp.Route {
		fmt.Printf(" %d: (%d,%d)\n", i, step.X, step.Y)
	}
}

func floorDiv(value, size int) int {
	if size <= 0 {
		return 0
	}
	if value >= 0 {
		return value / size
	}
	return -((-value - 1) / size) - 1
}
