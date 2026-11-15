package netutil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

const defaultMagicDNSAddr = "100.100.100.100:53"

// tailscaleCGNATNet is Tailscale's 100.64.0.0/10 address range.
var tailscaleCGNATNet = &net.IPNet{
	IP:   net.IPv4(100, 64, 0, 0),
	Mask: net.CIDRMask(10, 32),
}

// MagicDNSDialer wraps net.Dialer and falls back to Tailscale MagicDNS (100.100.100.100:53)
// when system DNS resolution fails (e.g., Docker containers started before tailscaled updates /etc/resolv.conf).
type MagicDNSDialer struct {
	baseDialer net.Dialer
	magicDNS   string
}

// NewDialer returns a MagicDNSDialer configured with the given dial timeout and keep-alive interval.
func NewDialer(timeout, keepAlive time.Duration) *MagicDNSDialer {
	return &MagicDNSDialer{
		baseDialer: net.Dialer{
			Timeout:   timeout,
			KeepAlive: keepAlive,
		},
		magicDNS: defaultMagicDNSAddr,
	}
}

// DialContextFunc returns a DialContext function suitable for http.Transport.
func DialContextFunc(timeout, keepAlive time.Duration) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return NewDialer(timeout, keepAlive).DialContext
}

// DialTimeout connects to addr on the named network with a timeout and MagicDNS fallback.
func DialTimeout(network, addr string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return NewDialer(timeout, 15*time.Second).DialContext(ctx, network, addr)
}

// NewHTTPClient returns an http.Client whose Transport uses the MagicDNS-aware dialer.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:       http.ProxyFromEnvironment,
			DialContext: DialContextFunc(5*time.Second, 15*time.Second),
		},
	}
}

// DialContext dials network/addr using the system resolver first, falling back to MagicDNS
// if a DNS lookup error occurs for a Tailscale hostname or on a host with a Tailscale interface.
func (d *MagicDNSDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := d.baseDialer.DialContext(ctx, network, addr)
	if err == nil {
		return conn, nil
	}

	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		return nil, err
	}

	host, port, splitErr := net.SplitHostPort(addr)
	if splitErr != nil || net.ParseIP(host) != nil {
		return nil, err
	}

	magicHost, _, _ := net.SplitHostPort(d.magicDNS)
	if magicHost != "" && strings.HasPrefix(dnsErr.Server, magicHost) {
		return nil, err
	}

	if !isTailscaleHostname(host) && !hasTailscaleInterface() {
		return nil, err
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(rCtx context.Context, _, _ string) (net.Conn, error) {
			rd := net.Dialer{Timeout: 2 * time.Second}
			return rd.DialContext(rCtx, "udp", d.magicDNS)
		},
	}

	ips, lookupErr := resolver.LookupIPAddr(lookupCtx, host)
	if lookupErr != nil || len(ips) == 0 {
		return nil, err
	}

	var lastErr error
	for _, ip := range ips {
		c, dialErr := d.baseDialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if dialErr == nil {
			return c, nil
		}
		lastErr = dialErr
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, err
}

func isTailscaleHostname(host string) bool {
	trimmed := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return strings.HasSuffix(trimmed, ".ts.net")
}

func hasTailscaleInterface() bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
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
			if ip4 := ip.To4(); ip4 != nil && tailscaleCGNATNet.Contains(ip4) {
				return true
			}
		}
	}
	return false
}

