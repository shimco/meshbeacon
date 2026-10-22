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

// JellyfinServerInfo represents the JSON payload sent in response to Jellyfin discovery probes.
type JellyfinServerInfo struct {
	Address string `json:"Address"`
	Id      string `json:"Id"`
	Name    string `json:"Name"`
}

// JellyfinBeacon handles listening for UDP 7359 broadcast queries.
type JellyfinBeacon struct {
	port int
	info JellyfinServerInfo
}

// NewJellyfinBeacon creates a new Jellyfin discovery responder.
func NewJellyfinBeacon(port int, advertiseAddr, serverId, serverName string) *JellyfinBeacon {
	return &JellyfinBeacon{
		port: port,
		info: JellyfinServerInfo{
			Address: advertiseAddr,
			Id:      serverId,
			Name:    serverName,
		},
	}
}

// Name returns the protocol identifier.
func (b *JellyfinBeacon) Name() string {
	return "Jellyfin UDP Broadcast"
}

// Start begins listening for discovery broadcasts on 0.0.0.0:port until ctx is canceled.
func (b *JellyfinBeacon) Start(ctx context.Context) error {
	addr := net.UDPAddr{
		Port: b.port,
		IP:   net.ParseIP("0.0.0.0"),
	}

	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return fmt.Errorf("failed to bind Jellyfin UDP port %d: %w", b.port, err)
	}
	defer conn.Close()

	payload, err := json.Marshal(b.info)
	if err != nil {
		return fmt.Errorf("failed to marshal server info: %w", err)
	}

	log.Printf("[Jellyfin Beacon] Listening on UDP %d, advertising %s (%s)", b.port, b.info.Address, b.info.Name)

	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[Jellyfin Beacon] Shutting down...")
			return nil
		default:
		}

		// Set read deadline so socket doesn't block forever, allowing clean context checks
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

		msg := string(buf[:n])
		if strings.Contains(msg, "Who is JellyfinServer?") {
			log.Printf("[Jellyfin Beacon] Probe from %s -> replying with %s", remoteAddr.String(), b.info.Address)
			if _, err := conn.WriteTo(payload, remoteAddr); err != nil {
				log.Printf("[Jellyfin Beacon] Write error: %v", err)
			}
		}
	}
}
