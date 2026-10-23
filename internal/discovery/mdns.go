package discovery

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/shimco/meshbeacon/internal/netutil"
)

// MDNSBeacon implements a native, zero-dependency Multicast DNS (mDNS / Bonjour / DNS-SD)
// responder and broadcaster conforming to RFC 6762 and RFC 6763.
// It announces media services on 224.0.0.251:5353 so Smart TVs (Apple TV, Android TV,
// Fire TV, Roku, Swiftfin, Infuse) autodiscover services without requiring third-party libraries.
type MDNSBeacon struct {
	instanceName string
	serviceType  string // e.g. "_jellyfin._tcp" or "_plexmediasvr._tcp"
	port         int
	txtRecords   []string
}

// NewMDNSBeacon creates an mDNS responder for a specific service type.
func NewMDNSBeacon(instanceName, serviceType string, port int, txtRecords []string) *MDNSBeacon {
	return &MDNSBeacon{
		instanceName: instanceName,
		serviceType:  serviceType,
		port:         port,
		txtRecords:   txtRecords,
	}
}

// Name returns the protocol identifier.
func (b *MDNSBeacon) Name() string {
	return fmt.Sprintf("mDNS (%s)", b.serviceType)
}

// Start runs active multicast announcements and listens for incoming queries until ctx is canceled.
func (b *MDNSBeacon) Start(ctx context.Context) error {
	mcastAddr, err := net.ResolveUDPAddr("udp4", "224.0.0.251:5353")
	if err != nil {
		return fmt.Errorf("failed to resolve mDNS multicast address: %w", err)
	}

	// Determine host identity and IP addresses
	hostName := getSanitizedHostname()
	ips := getLocalIPv4s()
	if len(ips) == 0 {
		outIP, err := netutil.GetOutboundIP()
		if err == nil {
			if parsed := net.ParseIP(outIP); parsed != nil {
				ips = append(ips, parsed)
			}
		}
	}

	// Pre-generate standard announcement and goodbye packets
	announcementPacket := b.buildDNSResponse(hostName, ips, false)
	goodbyePacket := b.buildDNSResponse(hostName, ips, true)

	// Create dedicated outbound UDP connection for sending multicasts
	outConn, err := net.DialUDP("udp4", nil, mcastAddr)
	if err != nil {
		return fmt.Errorf("failed to open outbound mDNS socket: %w", err)
	}
	defer outConn.Close()

	sendAnnouncement := func() {
		if _, err := outConn.Write(announcementPacket); err != nil {
			// Best effort multicast
			return
		}
	}

	// Listen for incoming queries if possible (using SO_REUSEPORT)
	queryConn, err := net.ListenMulticastUDP("udp4", nil, mcastAddr)
	if err == nil {
		defer queryConn.Close()
		go b.listenForQueries(ctx, queryConn, outConn, announcementPacket)
		log.Printf("[mDNS Beacon] Active & query-listening on 224.0.0.251:5353 for %s (%s)", b.instanceName, b.serviceType)
	} else {
		log.Printf("[mDNS Beacon] Note: Port 5353 bound by another service (%v); running active announcement broadcasts", err)
	}

	// 1. Startup burst: 0s, 2s, 5s, 10s to ensure aggressive discovery
	sendAnnouncement()
	for _, delay := range []time.Duration{2 * time.Second, 3 * time.Second, 5 * time.Second} {
		select {
		case <-ctx.Done():
			break
		case <-time.After(delay):
			sendAnnouncement()
		}
	}

	// 2. Steady-state loop: refresh every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[mDNS Beacon] Stopping broadcast for %s (sending goodbye packet)", b.serviceType)
			_, _ = outConn.Write(goodbyePacket)
			return nil
		case <-ticker.C:
			sendAnnouncement()
		}
	}
}

// listenForQueries reads incoming mDNS queries and immediately replies if our service is requested.
func (b *MDNSBeacon) listenForQueries(ctx context.Context, inConn *net.UDPConn, outConn *net.UDPConn, replyPacket []byte) {
	buf := make([]byte, 4096)
	targetService := strings.ToLower(b.serviceType + ".local")
	targetEnum := "_services._dns-sd._udp.local"
	targetInstance := strings.ToLower(b.instanceName + "." + b.serviceType + ".local")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_ = inConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := inConn.ReadFrom(buf)
		if err != nil {
			continue
		}

		if n < 12 {
			continue // DNS header is minimum 12 bytes
		}

		// Check if it's a standard query (QR bit == 0)
		flags := binary.BigEndian.Uint16(buf[2:4])
		if flags&0x8000 != 0 {
			continue // ignore responses / announcements from other hosts
		}

		qdCount := int(binary.BigEndian.Uint16(buf[4:6]))
		if qdCount == 0 {
			continue
		}

		// Inspect questions to see if our service is being queried
		offset := 12
		matched := false
		for q := 0; q < qdCount && offset < n; q++ {
			qname, nextOffset, err := parseDomainName(buf[:n], offset)
			if err != nil {
				break
			}
			offset = nextOffset + 4 // skip QTYPE (2 bytes) + QCLASS (2 bytes)

			qnameLower := strings.ToLower(qname)
			if qnameLower == targetService || qnameLower == targetEnum || qnameLower == targetInstance {
				matched = true
				break
			}
		}

		if matched {
			_, _ = outConn.Write(replyPacket)
		}
	}
}

// buildDNSResponse constructs a standard RFC 6762 / RFC 6763 DNS answer packet.
func (b *MDNSBeacon) buildDNSResponse(hostName string, ips []net.IP, isGoodbye bool) []byte {
	ttl := uint32(4500)
	hostTTL := uint32(120)
	if isGoodbye {
		ttl = 0
		hostTTL = 0
	}

	// Service labels
	// e.g. serviceType "_jellyfin._tcp" -> ["_jellyfin", "_tcp", "local"]
	typeParts := strings.Split(b.serviceType, ".")
	var srvLabels []string
	for _, p := range typeParts {
		if p != "" {
			srvLabels = append(srvLabels, p)
		}
	}
	srvLabels = append(srvLabels, "local")

	instLabels := append([]string{b.instanceName}, srvLabels...)
	hostLabels := []string{hostName, "local"}
	enumLabels := []string{"_services", "_dns-sd", "_udp", "local"}

	srvDomain := encodeDomain(srvLabels...)
	instDomain := encodeDomain(instLabels...)
	hostDomain := encodeDomain(hostLabels...)
	enumDomain := encodeDomain(enumLabels...)

	var buf bytes.Buffer

	// Answers: 1 DNS-SD Enum PTR + 1 Service PTR + 1 SRV + 1 TXT + len(ips) A records
	numAnswers := uint16(4 + len(ips))

	// DNS Header (12 bytes)
	binary.Write(&buf, binary.BigEndian, uint16(0))      // ID
	binary.Write(&buf, binary.BigEndian, uint16(0x8400)) // Flags: QR=1 (response), AA=1 (authoritative)
	binary.Write(&buf, binary.BigEndian, uint16(0))      // QDCOUNT
	binary.Write(&buf, binary.BigEndian, numAnswers)     // ANCOUNT
	binary.Write(&buf, binary.BigEndian, uint16(0))      // NSCOUNT
	binary.Write(&buf, binary.BigEndian, uint16(0))      // ARCOUNT

	// 1. DNS-SD Service Enumeration PTR: _services._dns-sd._udp.local -> _service._tcp.local
	buf.Write(enumDomain)
	binary.Write(&buf, binary.BigEndian, uint16(12)) // TYPE PTR
	binary.Write(&buf, binary.BigEndian, uint16(1))  // CLASS IN (shared)
	binary.Write(&buf, binary.BigEndian, ttl)
	binary.Write(&buf, binary.BigEndian, uint16(len(srvDomain)))
	buf.Write(srvDomain)

	// 2. Service PTR: _service._tcp.local -> Instance._service._tcp.local
	buf.Write(srvDomain)
	binary.Write(&buf, binary.BigEndian, uint16(12)) // TYPE PTR
	binary.Write(&buf, binary.BigEndian, uint16(1))  // CLASS IN (shared)
	binary.Write(&buf, binary.BigEndian, ttl)
	binary.Write(&buf, binary.BigEndian, uint16(len(instDomain)))
	buf.Write(instDomain)

	// 3. SRV: Instance._service._tcp.local -> hostName.local:port
	buf.Write(instDomain)
	binary.Write(&buf, binary.BigEndian, uint16(33))     // TYPE SRV
	binary.Write(&buf, binary.BigEndian, uint16(0x8001)) // CLASS IN + Cache Flush bit
	binary.Write(&buf, binary.BigEndian, hostTTL)
	srvRDataLen := uint16(6 + len(hostDomain))
	binary.Write(&buf, binary.BigEndian, srvRDataLen)
	binary.Write(&buf, binary.BigEndian, uint16(0))      // Priority
	binary.Write(&buf, binary.BigEndian, uint16(0))      // Weight
	binary.Write(&buf, binary.BigEndian, uint16(b.port)) // Port
	buf.Write(hostDomain)

	// 4. TXT: Instance._service._tcp.local -> Key-value metadata
	buf.Write(instDomain)
	binary.Write(&buf, binary.BigEndian, uint16(16))     // TYPE TXT
	binary.Write(&buf, binary.BigEndian, uint16(0x8001)) // CLASS IN + Cache Flush bit
	binary.Write(&buf, binary.BigEndian, ttl)
	var txtData []byte
	for _, t := range b.txtRecords {
		if len(t) > 0 {
			txtData = append(txtData, byte(len(t)))
			txtData = append(txtData, t...)
		}
	}
	if len(txtData) == 0 {
		txtData = []byte{0} // Single zero byte for empty TXT
	}
	binary.Write(&buf, binary.BigEndian, uint16(len(txtData)))
	buf.Write(txtData)

	// 5. A Records: hostName.local -> IPv4 addresses
	for _, ip := range ips {
		ip4 := ip.To4()
		if ip4 == nil {
			continue
		}
		buf.Write(hostDomain)
		binary.Write(&buf, binary.BigEndian, uint16(1))      // TYPE A
		binary.Write(&buf, binary.BigEndian, uint16(0x8001)) // CLASS IN + Cache Flush bit
		binary.Write(&buf, binary.BigEndian, hostTTL)
		binary.Write(&buf, binary.BigEndian, uint16(4)) // IPv4 length
		buf.Write(ip4)
	}

	return buf.Bytes()
}

// encodeDomain builds wire-format domain labels without compression pointers.
func encodeDomain(labels ...string) []byte {
	var buf []byte
	for _, l := range labels {
		if len(l) == 0 {
			continue
		}
		buf = append(buf, byte(len(l)))
		buf = append(buf, l...)
	}
	buf = append(buf, 0x00)
	return buf
}

// parseDomainName extracts a domain name from wire format, following compression pointers if present.
func parseDomainName(data []byte, offset int) (string, int, error) {
	var labels []string
	curr := offset
	jumped := false
	nextOffset := offset
	maxJumps := 10
	jumps := 0

	for {
		if curr >= len(data) {
			return "", 0, fmt.Errorf("truncated label")
		}
		length := int(data[curr])
		if length == 0 {
			if !jumped {
				nextOffset = curr + 1
			}
			break
		}
		if (length & 0xC0) == 0xC0 {
			if curr+1 >= len(data) {
				return "", 0, fmt.Errorf("truncated pointer")
			}
			if jumps > maxJumps {
				return "", 0, fmt.Errorf("too many compression jumps")
			}
			ptr := int(length&0x3F)<<8 | int(data[curr+1])
			if !jumped {
				nextOffset = curr + 2
				jumped = true
			}
			curr = ptr
			jumps++
			continue
		}
		curr++
		if curr+length > len(data) {
			return "", 0, fmt.Errorf("label length out of range")
		}
		labels = append(labels, string(data[curr:curr+length]))
		curr += length
	}

	return strings.Join(labels, "."), nextOffset, nil
}

// getSanitizedHostname gets the machine hostname or defaults to "meshbeacon".
func getSanitizedHostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "meshbeacon"
	}
	// Strip local domain suffix if present
	name = strings.TrimSuffix(name, ".local")
	// Sanitize characters
	var clean strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			clean.WriteRune(r)
		}
	}
	res := clean.String()
	if res == "" {
		return "meshbeacon"
	}
	return strings.ToLower(res)
}

// getLocalIPv4s retrieves all active non-loopback IPv4 addresses across network interfaces.
func getLocalIPv4s() []net.IP {
	var list []net.IP
	ifaces, err := net.Interfaces()
	if err != nil {
		return list
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				list = append(list, ip.To4())
			}
		}
	}
	return list
}
