package netutil

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestIsTailscaleHostname(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"mediaserver.horse-pancake.ts.net", true},
		{"MEDIASERVER.HORSE-PANCAKE.TS.NET.", true},
		{"example.com", false},
		{"ts.net.example.com", false},
		{"127.0.0.1", false},
		{"", false},
	}

	for _, tc := range tests {
		if got := isTailscaleHostname(tc.host); got != tc.want {
			t.Errorf("isTailscaleHostname(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestMagicDNSDialerFallback(t *testing.T) {
	// Start a local TCP listener to act as the target upstream service.
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start TCP listener: %v", err)
	}
	defer tcpLn.Close()

	go func() {
		for {
			c, err := tcpLn.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	_, port, err := net.SplitHostPort(tcpLn.Addr().String())
	if err != nil {
		t.Fatalf("failed to split listener addr: %v", err)
	}

	// Start a minimal UDP DNS responder returning 127.0.0.1 for any A query.
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start UDP DNS listener: %v", err)
	}
	defer udpConn.Close()

	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := udpConn.ReadFrom(buf)
			if err != nil || n < 12 {
				return
			}
			// Build minimal DNS response: copy ID, set QR=1 RD=1 RA=1, QDCOUNT=1, ANCOUNT=1
			resp := make([]byte, 0, n+16)
			resp = append(resp, buf[0], buf[1], 0x81, 0x80, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00)
			// Copy question section
			resp = append(resp, buf[12:n]...)
			// Answer: pointer to name (0xC00C), type A (1), class IN (1), TTL 60, RDLENGTH 4, 127.0.0.1
			resp = append(resp, 0xC0, 0x0C, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x3C, 0x00, 0x04, 127, 0, 0, 1)
			_, _ = udpConn.WriteTo(resp, addr)
		}
	}()

	d := &MagicDNSDialer{
		baseDialer: net.Dialer{Timeout: 2 * time.Second},
		magicDNS:   udpConn.LocalAddr().String(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Non-existent .ts.net host fails system DNS, then resolves to 127.0.0.1 via mock MagicDNS.
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort("nonexistent-test-node.invalid.ts.net", port))
	if err != nil {
		t.Fatalf("expected fallback dial to succeed, got error: %v", err)
	}
	_ = conn.Close()
}

