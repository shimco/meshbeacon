package logring

import (
	"fmt"
	"sync"
	"testing"
)

func TestRingBufferCapacity(t *testing.T) {
	rb := New(5)

	for i := 1; i <= 10; i++ {
		_, _ = rb.Write([]byte(fmt.Sprintf("log line %d\n", i)))
	}

	lines := rb.Lines()
	if len(lines) != 5 {
		t.Fatalf("Expected 5 lines, got %d", len(lines))
	}

	if lines[4] != "log line 10" && !contains(lines[4], "log line 10") {
		t.Errorf("Expected last line to be 'log line 10', got: %s", lines[4])
	}
}

func TestRingBufferConcurrentWrites(t *testing.T) {
	rb := New(50)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, _ = rb.Write([]byte(fmt.Sprintf("worker %d item %d\n", worker, j)))
			}
		}(i)
	}

	wg.Wait()
	lines := rb.Lines()
	if len(lines) > 50 {
		t.Errorf("Expected at most 50 lines, got %d", len(lines))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s[len(s)-len(substr):] == substr)
}
