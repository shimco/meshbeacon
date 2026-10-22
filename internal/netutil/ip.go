// Package netutil provides network utility functions for dynamic IP discovery and health checks.
package netutil

import (
	"errors"
	"net"
	"strings"
)

// GetOutboundIP dynamically determines the preferred local outbound IP of this machine.
// It uses an outbound UDP probe towards a dummy address without transmitting any packets.
func GetOutboundIP() (string, error) {
	// Probe route towards common public DNS (no actual packet is transmitted by DialUDP)
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		if localAddr.IP != nil && !localAddr.IP.IsLoopback() {
			return localAddr.IP.String(), nil
		}
	}

	// Fallback: iterate active physical network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue // skip down or loopback interfaces
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
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			return ip.String(), nil
		}
	}

	return "", errors.New("no valid outbound IPv4 interface found")
}

// NormalizeHost ensures the advertised address includes http:// and valid IP.
func NormalizeHost(host string) (string, error) {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" || trimmed == "auto" {
		ip, err := GetOutboundIP()
		if err != nil {
			return "", err
		}
		return "http://" + ip, nil
	}

	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		trimmed = "http://" + trimmed
	}
	return trimmed, nil
}
