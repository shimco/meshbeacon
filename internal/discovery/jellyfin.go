package discovery

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
)

// JellyfinServerInfo represents the JSON payload sent in response to Jellyfin discovery probes.
type JellyfinServerInfo struct {
	Address string `json:"Address"`
	Id      string `json:"Id"`
	Name    string `json:"Name"`
}

// JellyfinBeacon handles listening for UDP 7359 broadcast queries.
type JellyfinBeacon struct {
	port     int
	info     JellyfinServerInfo
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewJellyfinBeacon creates a new discovery responder.
func NewJellyfinBeacon(port int, advertiseAddr, serverId, serverName string) *JellyfinBeacon {
	return &JellyfinBeacon{
		port: port,
		info: JellyfinServerInfo{
			Address: advertiseAddr,
			Id:      serverId,
			Name:    serverName,
		},
		stopChan: make(chan struct{}),
	}
}

// Start begins listening for discovery broadcasts on 0.0.0.0:port.
func (b *JellyfinBeacon) Start() error {
	addr := net.UDPAddr{
		Port: b.port,
		IP:   net.ParseIP("0.0.0.0"),
	}

	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return fmt.Errorf("failed to bind UDP port %d: %w", b.port, err)
	}

	payload, err := json.Marshal(b.info)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to marshal server info: %w", err)
	}

	log.Printf("[Jellyfin Beacon] Listening on UDP %d, advertising %s (%s)", b.port, b.info.Address, b.info.Name)

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer conn.Close()

		buf := make([]byte, 1024)
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
					log.Printf("[Jellyfin Beacon] Read error: %v", err)
					continue
				}
			}

			msg := string(buf[:n])
			if strings.Contains(msg, "Who is JellyfinServer?") {
				log.Printf("[Jellyfin Beacon] Received discovery probe from %s -> responding", remoteAddr.String())
				if _, err := conn.WriteTo(payload, remoteAddr); err != nil {
					log.Printf("[Jellyfin Beacon] Write error: %v", err)
				}
			}
		}
	}()

	return nil
}

// Stop terminates the UDP listener.
func (b *JellyfinBeacon) Stop() {
	close(b.stopChan)
	b.wg.Wait()
	log.Printf("[Jellyfin Beacon] Stopped")
}
