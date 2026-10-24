package proxy

import (
	"context"
	"testing"
	"time"
)

func TestTelemetryRecordAndStats(t *testing.T) {
	tel := NewTelemetry(false)

	tel.Record("jellyfin", "192.168.1.50", "GET", "/Items/1/Images/Primary", "HIT")
	tel.Record("jellyfin", "192.168.1.50", "GET", "/Items/2/Images/Primary", "MISS")
	tel.Record("plex", "192.168.1.60", "GET", "/video/stream.m3u8", "STREAM")

	total, hits, misses, streams, ratio := tel.Stats()
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if hits != 1 {
		t.Errorf("expected hits 1, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected misses 1, got %d", misses)
	}
	if streams != 1 {
		t.Errorf("expected streams 1, got %d", streams)
	}
	if ratio != 50.0 {
		t.Errorf("expected ratio 50.0%%, got %.1f%%", ratio)
	}

	recent := tel.Recent()
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent traces, got %d", len(recent))
	}
}

func TestTelemetryRingBufferCapacity(t *testing.T) {
	tel := NewTelemetry(false)
	tel.capacity = 3 // small capacity for test

	tel.Record("plex", "1.1.1.1", "GET", "/1", "HIT")
	tel.Record("plex", "1.1.1.1", "GET", "/2", "HIT")
	tel.Record("plex", "1.1.1.1", "GET", "/3", "HIT")
	tel.Record("plex", "1.1.1.1", "GET", "/4", "HIT")

	recent := tel.Recent()
	if len(recent) != 3 {
		t.Fatalf("expected capped length 3, got %d", len(recent))
	}
}

func TestTelemetryHeartbeat(t *testing.T) {
	tel := NewTelemetry(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tel.StartHeartbeat(ctx, 10*time.Millisecond)
	tel.Record("jellyfin", "192.168.1.50", "GET", "/img", "HIT")

	time.Sleep(25 * time.Millisecond)
	// Verify lastTotal advanced
	tel.mu.RLock()
	advanced := tel.lastTotal == 1
	tel.mu.RUnlock()

	if !advanced {
		t.Errorf("expected lastTotal to advance after heartbeat tick")
	}
}
