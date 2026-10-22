package discovery

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// PlexGDMBeacon handles listening for Plex Good Day Mate (GDM) UDP discovery broadcasts on port 32414.
type PlexGDMBeacon struct {
	port       int
	serverName string
	serverPort int
	resourceId string
}

// NewPlexGDMBeacon creates a new Plex GDM discovery responder.
func NewPlexGDMBeacon(port int, serverName string, serverPort int, resourceId string) *PlexGDMBeacon {
	return &PlexGDMBeacon{
		port:       port,
		serverName: serverName,
		serverPort: serverPort,
		resourceId: resourceId,
	}
}

// Name returns the protocol identifier.
func (b *PlexGDMBeacon) Name() string {
	return "Plex GDM Broadcast"
}

// Start begins listening on UDP port for GDM search queries until ctx is canceled.
func (b *PlexGDMBeacon) Start(ctx context.Context) error {
	addr := net.UDPAddr{
		Port: b.port,
		IP:   net.ParseIP("0.0.0.0"),
	}

	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return fmt.Errorf("failed to bind Plex GDM UDP port %d: %w", b.port, err)
	}
	defer conn.Close()

	log.Printf("[Plex Beacon] Listening on UDP %d (GDM), advertising '%s' on port %d", b.port, b.serverName, b.serverPort)

	buf := make([]byte, 2048)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[Plex Beacon] Shutting down...")
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
				continue
			}
		}

		msg := string(buf[:n])
		if strings.Contains(msg, "M-SEARCH") {
			log.Printf("[Plex Beacon] GDM probe from %s -> responding", remoteAddr.String())

			resp := fmt.Sprintf(
				"HTTP/1.0 200 OK\r\n"+
					"Content-Type: plex/media-server\r\n"+
					"Name: %s\r\n"+
					"Port: %d\r\n"+
					"Resource-Identifier: %s\r\n"+
					"Version: 1.40.0\r\n"+
					"Updated-At: %d\r\n\r\n",
				b.serverName,
				b.serverPort,
				b.resourceId,
				time.Now().Unix(),
			)

			if _, err := conn.WriteTo([]byte(resp), remoteAddr); err != nil {
				log.Printf("[Plex Beacon] Write error: %v", err)
			}
		}
	}
}
