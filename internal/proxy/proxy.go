package proxy

import (
	"bytes"
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/shimco/meshbeacon/internal/cache"
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
	name      string
	target    *url.URL
	proxy     *httputil.ReverseProxy
	cache     *cache.DualTierCache
	telemetry *Telemetry
}

func clientIP(r *http.Request) string {
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}
	return r.RemoteAddr
}

// NewStreamProxy initializes a hardened reverse proxy pointing to the target URL.
func NewStreamProxy(name, targetURL string, assetCache *cache.DualTierCache, tel *Telemetry) (*StreamProxy, error) {
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
		clientHost := req.Host
		if clientHost == "" {
			clientHost = req.Header.Get("Host")
		}
		originalDirector(req)
		req.Host = u.Host
		if clientHost != "" {
			req.Header.Set("X-Forwarded-Host", clientHost)
		}
		if cIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			req.Header.Set("X-Real-IP", cIP)
		}
	}

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[Proxy Error][%s] %s %s: %v", name, r.Method, r.URL.Path, err)
		http.Error(w, "Meshbeacon Gateway Error: Upstream service unavailable", http.StatusBadGateway)
	}

	return &StreamProxy{
		name:      name,
		target:    u,
		proxy:     rp,
		cache:     assetCache,
		telemetry: tel,
	}, nil
}

// ServeHTTP handles incoming requests, serving static artwork from cache and streaming video through buffers.
func (sp *StreamProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[Panic Recovered][%s] %s %s: %v", sp.name, r.Method, r.URL.Path, rec)
			http.Error(w, "Meshbeacon Gateway Error: Internal server error", http.StatusInternalServerError)
		}
	}()

	ip := clientIP(r)

	// Intercept static artwork requests if caching is enabled
	if sp.cache != nil && cache.IsCacheableAsset(sp.name, r.Method, r.URL.Path) {
		cacheKey := cache.CacheKey(sp.name, r.URL.Path, r.URL.RawQuery)

		// 1. Check Cache HIT
		if item, ok := sp.cache.Get(cacheKey); ok {
			sp.telemetry.Record(sp.name, ip, r.Method, r.URL.Path, "HIT")

			// Conditional GET via ETag (304 Not Modified)
			if match := r.Header.Get("If-None-Match"); match != "" && match == item.ETag {
				w.Header().Set("X-Meshbeacon-Cache", "HIT")
				w.WriteHeader(http.StatusNotModified)
				return
			}

			w.Header().Set("X-Meshbeacon-Cache", "HIT")
			if item.ContentType != "" {
				w.Header().Set("Content-Type", item.ContentType)
			}
			if item.ETag != "" {
				w.Header().Set("ETag", item.ETag)
			}
			w.Header().Set("Cache-Control", "public, max-age=2592000") // 30 days
			w.Header().Set("Content-Length", strconv.FormatInt(item.Size, 10))
			_, _ = w.Write(item.Data)
			return
		}

		// 2. Cache MISS: stream upstream response through capture wrapper
		sp.telemetry.Record(sp.name, ip, r.Method, r.URL.Path, "MISS")

		crw := &cachingResponseWriter{
			ResponseWriter: w,
			maxCapture:     5 * 1024 * 1024, // 5MB max per image asset
		}

		sp.proxy.ServeHTTP(crw, r)

		// Store in cache if 200 OK and within size limit
		if !crw.tooLarge && crw.statusCode == http.StatusOK && crw.buf.Len() > 0 {
			contentType := crw.Header().Get("Content-Type")
			etag := crw.Header().Get("ETag")
			sp.cache.Set(cacheKey, crw.buf.Bytes(), contentType, etag)
		}
		return
	}

	// Direct stream proxy (video chunks, API calls, WebSockets)
	sp.telemetry.Record(sp.name, ip, r.Method, r.URL.Path, "STREAM")
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

// cachingResponseWriter captures binary bytes for caching while immediately streaming to the client.
type cachingResponseWriter struct {
	http.ResponseWriter
	buf         bytes.Buffer
	statusCode  int
	maxCapture  int64
	tooLarge    bool
	wroteHeader bool
}

func (crw *cachingResponseWriter) WriteHeader(code int) {
	if !crw.wroteHeader {
		crw.statusCode = code
		crw.wroteHeader = true
		crw.ResponseWriter.Header().Set("X-Meshbeacon-Cache", "MISS")
		crw.ResponseWriter.WriteHeader(code)
	}
}

func (crw *cachingResponseWriter) Write(b []byte) (int, error) {
	if !crw.wroteHeader {
		crw.WriteHeader(http.StatusOK)
	}
	n, err := crw.ResponseWriter.Write(b)
	if !crw.tooLarge && int64(crw.buf.Len()+n) <= crw.maxCapture {
		crw.buf.Write(b[:n])
	} else if !crw.tooLarge {
		crw.tooLarge = true
		crw.buf.Reset() // drop buffer to prevent wasting memory on huge non-image responses
	}
	return n, err
}

func (crw *cachingResponseWriter) Flush() {
	if f, ok := crw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
