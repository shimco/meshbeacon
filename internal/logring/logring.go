package logring

import (
	"strings"
	"sync"
	"time"
)

// RingBuffer stores the last N log lines in memory with zero disk I/O.
type RingBuffer struct {
	mu       sync.RWMutex
	lines    []string
	capacity int
}

// New creates a new in-memory log ring buffer.
func New(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 250
	}
	return &RingBuffer{
		capacity: capacity,
		lines:    make([]string, 0, capacity),
	}
}

// Write implements io.Writer so it can be passed to log.SetOutput or io.MultiWriter.
func (r *RingBuffer) Write(p []byte) (n int, err error) {
	str := string(p)
	rawLines := strings.Split(strings.TrimRight(str, "\r\n"), "\n")

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, l := range rawLines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		// If line doesn't already have timestamp, prefix with time
		if !strings.HasPrefix(trimmed, "20") && !strings.HasPrefix(trimmed, "[") {
			trimmed = time.Now().Format("15:04:05") + " " + trimmed
		}
		if len(r.lines) >= r.capacity {
			r.lines = r.lines[1:]
		}
		r.lines = append(r.lines, trimmed)
	}

	return len(p), nil
}

// Lines returns a thread-safe snapshot of all log lines currently held in memory.
func (r *RingBuffer) Lines() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, len(r.lines))
	copy(out, r.lines)
	return out
}

// Clear empties the ring buffer.
func (r *RingBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = make([]string, 0, r.capacity)
}
