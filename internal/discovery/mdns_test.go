package discovery

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
)

func TestEncodeAndParseDomainName(t *testing.T) {
	encoded := encodeDomain("Jellyfin Server", "_jellyfin", "_tcp", "local")
	parsed, offset, err := parseDomainName(encoded, 0)
	if err != nil {
		t.Fatalf("unexpected error parsing domain: %v", err)
	}
	expected := "Jellyfin Server._jellyfin._tcp.local"
	if parsed != expected {
		t.Errorf("expected %q, got %q", expected, parsed)
	}
	if offset != len(encoded) {
		t.Errorf("expected offset %d, got %d", len(encoded), offset)
	}
}

func TestBuildDNSResponseStructure(t *testing.T) {
	b := NewMDNSBeacon("Living Room", "_jellyfin._tcp", 8096, []string{"path=/"})
	ips := []net.IP{net.ParseIP("192.168.0.2")}

	packet := b.buildDNSResponse("test-host", ips, false)

	// Validate minimal length and header
	if len(packet) < 12 {
		t.Fatalf("packet too short: %d bytes", len(packet))
	}

	// Verify Header
	id := binary.BigEndian.Uint16(packet[0:2])
	if id != 0 {
		t.Errorf("expected ID 0, got %d", id)
	}
	flags := binary.BigEndian.Uint16(packet[2:4])
	if flags != 0x8400 {
		t.Errorf("expected flags 0x8400, got 0x%04x", flags)
	}
	anCount := binary.BigEndian.Uint16(packet[6:8])
	if anCount != 5 { // 1 DNS-SD Enum PTR + 1 Service PTR + 1 SRV + 1 TXT + 1 A
		t.Errorf("expected 5 answers, got %d", anCount)
	}

	// Test Goodbye Packet (TTL must be 0)
	goodbye := b.buildDNSResponse("test-host", ips, true)
	if len(goodbye) == 0 {
		t.Fatalf("goodbye packet empty")
	}
}

func TestParseCompressedDomain(t *testing.T) {
	// Build a buffer with a domain name and a pointer pointing to it
	// Name at offset 0: "local" (0x05, 'l','o','c','a','l', 0x00) -> 7 bytes
	// Second name at offset 7: "meshbeacon" + pointer to offset 0 (0xC0, 0x00)
	buf := []byte{
		0x05, 'l', 'o', 'c', 'a', 'l', 0x00,
		0x0a, 'm', 'e', 's', 'h', 'b', 'e', 'a', 'c', 'o', 'n',
		0xc0, 0x00,
	}

	name, nextOffset, err := parseDomainName(buf, 7)
	if err != nil {
		t.Fatalf("failed to parse compressed domain: %v", err)
	}
	expected := "meshbeacon.local"
	if name != expected {
		t.Errorf("expected %q, got %q", expected, name)
	}
	if nextOffset != len(buf) {
		t.Errorf("expected nextOffset %d, got %d", len(buf), nextOffset)
	}
}

func TestSanitizeHostname(t *testing.T) {
	name := getSanitizedHostname()
	if name == "" {
		t.Errorf("expected non-empty hostname")
	}
	if strings.Contains(name, ".local") {
		t.Errorf("hostname should not contain .local suffix")
	}
}

func TestSetCollisionPolicy(t *testing.T) {
	b := NewMDNSBeacon("Samba Share", "_smb._tcp", 445, []string{"path=/"})
	if b.collisionPolicy != "auto_rename" {
		t.Errorf("expected default collision policy 'auto_rename', got %q", b.collisionPolicy)
	}

	b.SetCollisionPolicy("suppress")
	if b.collisionPolicy != "suppress" {
		t.Errorf("expected collision policy 'suppress', got %q", b.collisionPolicy)
	}
}

func TestDaemonTXTRecord(t *testing.T) {
	b := NewMDNSBeacon("Jellyfin", "_jellyfin._tcp", 8096, []string{"path=/"})
	ips := []net.IP{net.ParseIP("192.168.1.100")}
	packet := b.buildDNSResponse("test-host", ips, false)

	// Verify that "daemon=meshbeacon" is present in the raw DNS packet
	if !strings.Contains(string(packet), "daemon=meshbeacon") {
		t.Errorf("expected packet to contain 'daemon=meshbeacon' in TXT record")
	}

	// For _device-info._tcp, verify daemon=meshbeacon is NOT injected (strict Apple CoreTypes compatibility)
	devInfo := NewMDNSBeacon("Media Server", "_device-info._tcp", 0, []string{"model=Xserve"})
	devPacket := devInfo.buildDNSResponse("test-host", ips, false)
	if strings.Contains(string(devPacket), "daemon=meshbeacon") {
		t.Errorf("_device-info._tcp should not contain 'daemon=meshbeacon' to preserve macOS CoreTypes icon rendering")
	}
}

