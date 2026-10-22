package discovery

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// PlexGDMBeacon handles listening for Plex Good Day Mate (GDM) UDP discovery broadcasts on port 32414.
type PlexGDMBeacon struct {
	port       int
	serverName string
	serverPort int
	resourceId string
	stopChan   chan struct{}
	wg         sync.WaitGroup
}

// NewPlexGDMBeacon creates a new Plex GDM discovery responder.
func NewPlexGDMBeacon(port int, serverName string, serverPort int, resourceId string) *PlexGDMBeacon {
	return &PlexGDMBeacon{
		port:       port,
		serverName: serverName,
		serverPort: serverPort,
		resourceId: resourceId,
		stopChan:   make(chan struct{}),
	}
}

// Start begins listening on UDP 32414 for GDM search queries.
func (b *PlexGDMBeacon) Start() error {
	addr := net.UDPAddr{
		Port: b.port,
		IP:   net.ParseIP("0.0.0.0"),
	}

	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return fmt.Errorf("failed to bind Plex GDM UDP port %d: %w", b.port, err)
	}

	log.Printf("[Plex Beacon] Listening on UDP %d (GDM), advertising '%s' on port %d", b.port, b.serverName, b.serverPort)

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer conn.Close()

		buf := make([]byte, 2048)
		for {
			select {
			case <-b.stopChan:
				return
			default:
			}

			n, remoteAddr, err := conn.ReadFrom(buf)
			if err != nil {
				select {
				case <-b.stopChan:
					return
				default:
					continue
				}
			}

			msg := string(buf[:n])
			if strings.Contains(msg, "M-SEARCH") {
				log.Printf("[Plex Beacon] Received GDM discovery probe from %s -> responding", remoteAddr.String())

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
	}()

	return nil
}

// Stop terminates the Plex GDM listener.
func (b *PlexGDMBeacon) Stop() {
	close(b.stopChan)
	b.wg.Wait()
	log.Printf("[Plex Beacon] Stopped")
}
