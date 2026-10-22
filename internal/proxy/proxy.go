package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// StreamProxy handles forwarding client media streams and API requests to the upstream Tailscale peer.
type StreamProxy struct {
	target *url.URL
	proxy  *httputil.ReverseProxy
}

// NewStreamProxy initializes a reverse proxy pointing to the target URL.
func NewStreamProxy(targetURL string) (*StreamProxy, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	rp := httputil.NewSingleHostReverseProxy(u)

	// Custom director to preserve host and setup forwarding headers
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = u.Host
		req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	}

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[Proxy Error] %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "Tailbeacon Gateway Error: Upstream service unreachable", http.StatusBadGateway)
	}

	return &StreamProxy{
		target: u,
		proxy:  rp,
	}, nil
}

// ServeHTTP implements http.Handler.
func (sp *StreamProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sp.proxy.ServeHTTP(w, r)
}
