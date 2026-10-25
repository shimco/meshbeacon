package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestTCPProxy(t *testing.T) {
	// 1. Mock upstream TCP echo server
	upstreamListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on upstream: %v", err)
	}
	defer upstreamListener.Close()

	upstreamAddr := upstreamListener.Addr().String()

	go func() {
		conn, err := upstreamListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Simple echo
		_, _ = io.Copy(conn, conn)
	}()

	// 2. Start TCPProxy on free random port
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	proxyPort := proxyListener.Addr().(*net.TCPAddr).Port
	_ = proxyListener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tp := NewTCPProxy("test-tcp", proxyPort, upstreamAddr)
	go func() {
		if err := tp.Start(ctx); err != nil {
			t.Logf("TCPProxy stopped: %v", err)
		}
	}()

	time.Sleep(50 * time.Millisecond)

	// 3. Connect client to TCPProxy and send data
	client, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer client.Close()

	message := "Hello Meshbeacon TCP Forwarder"
	if _, err := client.Write([]byte(message)); err != nil {
		t.Fatalf("failed to write to proxy: %v", err)
	}

	buf := make([]byte, 1024)
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("failed to read echoed data: %v", err)
	}

	if string(buf[:n]) != message {
		t.Fatalf("expected echo %q, got %q", message, string(buf[:n]))
	}
}
