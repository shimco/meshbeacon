package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// jellyfinPayload is the JSON payload sent in response to and in active broadcasts for Jellyfin discovery.
// Field names and structure must match exactly what the official Jellyfin server sends.
type jellyfinPayload struct {
	Address         string `json:"Address"`
	Id              string `json:"Id"`
	Name            string `json:"Name"`
	EndpointAddress string `json:"EndpointAddress"`
}

// JellyfinBeacon implements two-way Jellyfin UDP discovery on port 7359:
//  1. Passive: listens for "Who is JellyfinServer?" broadcasts and replies.
//  2. Active: periodically broadcasts server presence to 255.255.255.255:7359,
//     matching the real Jellyfin server's behavior so TVs find it without initiating a probe.
type JellyfinBeacon struct {
	port            int
	payload         []byte
	broadcastPeriod time.Duration
}

// NewJellyfinBeacon creates a new Jellyfin discovery responder.
// advertiseAddr must be the full HTTP URL, e.g. "http://192.168.100.2".
// The port is appended and the URL is normalized to always have a trailing slash.
func NewJellyfinBeacon(port int, advertiseAddr, serverId, serverName string) *JellyfinBeacon {
	// Normalize: ensure trailing slash on address (required by Jellyfin clients)
	addr := strings.TrimRight(advertiseAddr, "/") + "/"

	info := jellyfinPayload{
		Address:         addr,
		Id:              serverId,
		Name:            serverName,
		EndpointAddress: addr,
	}

	payload, _ := json.Marshal(info)

	return &JellyfinBeacon{
		port:            port,
		payload:         payload,
		broadcastPeriod: 30 * time.Second,
	}
}

// Name returns the protocol identifier.
func (b *JellyfinBeacon) Name() string {
	return "Jellyfin UDP Broadcast"
}

// Start runs both the passive probe listener and active broadcaster until ctx is canceled.
func (b *JellyfinBeacon) Start(ctx context.Context) error {
	listenAddr := net.UDPAddr{
		Port: b.port,
		IP:   net.ParseIP("0.0.0.0"),
	}

	conn, err := net.ListenUDP("udp4", &listenAddr)
	if err != nil {
		return fmt.Errorf("failed to bind Jellyfin UDP port %d: %w", b.port, err)
	}
	defer conn.Close()

	// Enable SO_BROADCAST so we can send to 255.255.255.255
	if err := conn.SetWriteBuffer(65536); err != nil {
		log.Printf("[Jellyfin Beacon] Warning: could not set write buffer: %v", err)
	}

	broadcastAddr := &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: b.port,
	}

	log.Printf("[Jellyfin Beacon] Listening on UDP %d (passive + active broadcast every %s)", b.port, b.broadcastPeriod)

	// Active broadcaster goroutine
	go func() {
		// Broadcast immediately on startup so TVs that are already on the screen see it
		b.broadcast(conn, broadcastAddr)

		ticker := time.NewTicker(b.broadcastPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.broadcast(conn, broadcastAddr)
			}
		}
	}()

	// Passive probe listener
	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[Jellyfin Beacon] Shutting down...")
			return nil
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, remoteAddr, err := conn.ReadFrom(buf)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			select {
			case <-ctx.Done():
				return nil
			default:
				log.Printf("[Jellyfin Beacon] Read error: %v", err)
				continue
			}
		}

		msg := strings.TrimSpace(string(buf[:n]))
		if strings.Contains(msg, "Who is JellyfinServer?") {
			log.Printf("[Jellyfin Beacon] Probe from %s -> replying", remoteAddr.String())
			if _, err := conn.WriteTo(b.payload, remoteAddr); err != nil {
				log.Printf("[Jellyfin Beacon] Reply error: %v", err)
			}
		}
	}
}

func (b *JellyfinBeacon) broadcast(conn *net.UDPConn, addr *net.UDPAddr) {
	if _, err := conn.WriteTo(b.payload, addr); err != nil {
		log.Printf("[Jellyfin Beacon] Broadcast error: %v", err)
	} else {
		log.Printf("[Jellyfin Beacon] Broadcast -> %s", addr.String())
	}
}
