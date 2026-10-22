package proxy

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// bufferPool implements httputil.BufferPool using sync.Pool for zero-allocation streaming.
type bufferPool struct {
	pool sync.Pool
}

func newBufferPool() *bufferPool {
	return &bufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				// 32 KB chunk size is standard for high-performance HTTP streaming
				b := make([]byte, 32*1024)
				return b
			},
		},
	}
}

func (bp *bufferPool) Get() []byte {
	return bp.pool.Get().([]byte)
}

func (bp *bufferPool) Put(b []byte) {
	bp.pool.Put(b)
}

// defaultBufferPool is shared across all streaming proxies.
var defaultBufferPool = newBufferPool()

// StreamProxy handles forwarding client media streams and API requests to the upstream Tailscale peer.
type StreamProxy struct {
	name   string
	target *url.URL
	proxy  *httputil.ReverseProxy
}

// NewStreamProxy initializes a hardened reverse proxy pointing to the target URL.
func NewStreamProxy(name, targetURL string) (*StreamProxy, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	// Hardened HTTP Transport with explicit timeouts to prevent connection hangs
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 15 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	rp := httputil.NewSingleHostReverseProxy(u)
	rp.Transport = transport
	rp.BufferPool = defaultBufferPool

	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = u.Host
		req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
		if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			req.Header.Set("X-Real-IP", clientIP)
		}
	}

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[Proxy Error][%s] %s %s: %v", name, r.Method, r.URL.Path, err)
		http.Error(w, "Meshbeacon Gateway Error: Upstream service unavailable", http.StatusBadGateway)
	}

	return &StreamProxy{
		name:   name,
		target: u,
		proxy:  rp,
	}, nil
}

// ServeHTTP implements http.Handler with basic latency and connection logging.
func (sp *StreamProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sp.proxy.ServeHTTP(w, r)
}

// Ping checks if the upstream target is responding.
func (sp *StreamProxy) Ping(ctx context.Context) error {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, sp.target.String(), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
