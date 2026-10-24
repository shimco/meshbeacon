package proxy

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Telemetry tracks gateway request metrics and maintains an in-memory circular
// trace buffer with zero persistent disk/flash wear on SD cards or SPI flash.
type Telemetry struct {
	mu            sync.RWMutex
	accessLog     bool
	totalRequests atomic.Uint64
	cacheHits     atomic.Uint64
	cacheMisses   atomic.Uint64
	directStreams atomic.Uint64

	lastTotal   uint64
	lastHits    uint64
	lastMisses  uint64
	lastStreams uint64

	ring     []string
	capacity int
}

// NewTelemetry creates a telemetry tracker. If accessLog is true, each incoming
// request is immediately output to stdout for interactive debugging.
func NewTelemetry(accessLog bool) *Telemetry {
	return &Telemetry{
		accessLog: accessLog,
		capacity:  50,
		ring:      make([]string, 0, 50),
	}
}

// Record logs a request into the RAM circular buffer and optionally outputs to stdout if accessLog is true.
func (t *Telemetry) Record(service, clientIP, method, path, status string) {
	if t == nil {
		return
	}

	t.totalRequests.Add(1)
	switch status {
	case "HIT":
		t.cacheHits.Add(1)
	case "MISS":
		t.cacheMisses.Add(1)
	case "STREAM":
		t.directStreams.Add(1)
	}

	trace := fmt.Sprintf("%s [%s] %s %s %s (%s)",
		time.Now().Format("15:04:05"), service, clientIP, method, path, status)

	t.mu.Lock()
	if len(t.ring) >= t.capacity {
		t.ring = t.ring[1:]
	}
	t.ring = append(t.ring, trace)
	t.mu.Unlock()

	if t.accessLog {
		log.Printf("[Gateway][%s] %s %s %s [%s]", service, clientIP, method, path, status)
	}
}

// Recent returns the last N request traces recorded in RAM.
func (t *Telemetry) Recent() []string {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	res := make([]string, len(t.ring))
	copy(res, t.ring)
	return res
}

// Stats returns current cumulative counters.
func (t *Telemetry) Stats() (total, hits, misses, streams uint64, hitRatio float64) {
	if t == nil {
		return 0, 0, 0, 0, 0
	}
	total = t.totalRequests.Load()
	hits = t.cacheHits.Load()
	misses = t.cacheMisses.Load()
	streams = t.directStreams.Load()

	cacheAttempts := hits + misses
	if cacheAttempts > 0 {
		hitRatio = (float64(hits) / float64(cacheAttempts)) * 100
	}
	return
}

// StartHeartbeat runs a periodic background rollup logger that emits at most one summary
// line every interval only if new traffic occurred (preventing SD-card and flash wear).
func (t *Telemetry) StartHeartbeat(ctx context.Context, interval time.Duration) {
	if t == nil {
		return
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.mu.Lock()
				curTotal := t.totalRequests.Load()
				curHits := t.cacheHits.Load()
				curMisses := t.cacheMisses.Load()
				curStreams := t.directStreams.Load()

				diffTotal := curTotal - t.lastTotal
				diffHits := curHits - t.lastHits
				diffMisses := curMisses - t.lastMisses
				diffStreams := curStreams - t.lastStreams

				if diffTotal > 0 {
					var ratio float64
					cacheAttempts := diffHits + diffMisses
					if cacheAttempts > 0 {
						ratio = (float64(diffHits) / float64(cacheAttempts)) * 100
					}

					log.Printf("[Gateway Telemetry] Last %s: %d reqs (%d hits, %d misses, %.1f%% cache hit ratio), %d streaming chunks",
						interval, diffTotal, diffHits, diffMisses, ratio, diffStreams)

					t.lastTotal = curTotal
					t.lastHits = curHits
					t.lastMisses = curMisses
					t.lastStreams = curStreams
				}
				t.mu.Unlock()
			}
		}
	}()
}
