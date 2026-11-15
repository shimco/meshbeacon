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
			if err != nil || n < 17 {
				return
			}
			// Find end of QNAME (0x00 terminator) starting at offset 12.
			qEnd := 12
			for qEnd < n && buf[qEnd] != 0 {
				qEnd += int(buf[qEnd]) + 1
			}
			// Question ends after 0x00 + QTYPE (2) + QCLASS (2).
			if qEnd+5 > n {
				continue
			}
			qType := uint16(buf[qEnd+1])<<8 | uint16(buf[qEnd+2])
			question := buf[12 : qEnd+5]

			// Header: ID, QR=1 RD=1 RA=1, QDCOUNT=1, ANCOUNT=1 (for A) or 0 (for AAAA), NSCOUNT=0, ARCOUNT=0
			anCount := byte(0)
			if qType == 1 {
				anCount = 1
			}
			resp := make([]byte, 0, len(question)+32)
			resp = append(resp, buf[0], buf[1], 0x81, 0x80, 0x00, 0x01, 0x00, anCount, 0x00, 0x00, 0x00, 0x00)
			resp = append(resp, question...)
			if qType == 1 {
				// Answer: pointer to name (0xC00C), type A (1), class IN (1), TTL 60, RDLENGTH 4, 127.0.0.1
				resp = append(resp, 0xC0, 0x0C, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x3C, 0x00, 0x04, 127, 0, 0, 1)
			}
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

