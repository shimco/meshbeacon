package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// TCPProxy forwards raw TCP streams (e.g. SMB port 445, SSH, raw sockets) to an upstream target.
type TCPProxy struct {
	name       string
	localPort  int
	targetAddr string
	listener   net.Listener
}

// NewTCPProxy creates a raw TCP stream forwarder.
func NewTCPProxy(name string, localPort int, targetAddr string) *TCPProxy {
	return &TCPProxy{
		name:       name,
		localPort:  localPort,
		targetAddr: targetAddr,
	}
}

// Start listens on the local TCP port and forwards bidirectional streams until ctx is canceled.
func (tp *TCPProxy) Start(ctx context.Context) error {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", tp.localPort))
	if err != nil {
		return fmt.Errorf("tcp proxy [%s] failed to bind :%d: %w", tp.name, tp.localPort, err)
	}
	tp.listener = l
	log.Printf("[TCP Gateway] %s stream proxy listening on :%d -> %s", tp.name, tp.localPort, tp.targetAddr)

	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()

	for {
		clientConn, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				continue
			}
		}

		go tp.handleConn(clientConn)
	}
}

func (tp *TCPProxy) handleConn(src net.Conn) {
	defer src.Close()

	dst, err := net.DialTimeout("tcp", tp.targetAddr, 5*time.Second)
	if err != nil {
		log.Printf("[TCP Proxy Error][%s] failed to connect to upstream %s: %v", tp.name, tp.targetAddr, err)
		return
	}
	defer dst.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if tcpDst, ok := dst.(*net.TCPConn); ok {
			_ = tcpDst.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(src, dst)
		if tcpSrc, ok := src.(*net.TCPConn); ok {
			_ = tcpSrc.CloseWrite()
		}
	}()

	wg.Wait()
}
