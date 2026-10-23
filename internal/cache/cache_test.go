package cache

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestCalculateAutoRAMCacheSize(t *testing.T) {
	tests := []struct {
		ram      int64
		expected int64
	}{
		{ram: 64 * MB, expected: 8 * MB},
		{ram: 128 * MB, expected: 8 * MB},
		{ram: 256 * MB, expected: 32 * MB},
		{ram: 512 * MB, expected: 32 * MB},
		{ram: 1 * GB, expected: 96 * MB},
		{ram: 2 * GB, expected: 192 * MB},
		{ram: 4 * GB, expected: 256 * MB},
		{ram: 16 * GB, expected: 512 * MB},
		{ram: 64 * GB, expected: 512 * MB},
	}

	for _, tc := range tests {
		actual := CalculateAutoRAMCacheSize(tc.ram)
		if actual != tc.expected {
			t.Errorf("For RAM %s, expected cache size %s, got %s",
				FormatSize(tc.ram), FormatSize(tc.expected), FormatSize(actual))
		}
	}
}

func TestParseSize(t *testing.T) {
	autoVal := int64(96 * MB)

	tests := []struct {
		input    string
		expected int64
	}{
		{"auto", autoVal},
		{"AUTO", autoVal},
		{"off", 0},
		{"OFF", 0},
		{"0", 0},
		{"disabled", 0},
		{"64MB", 64 * MB},
		{"64mb", 64 * MB},
		{"128M", 128 * MB},
		{"1GB", 1 * GB},
		{"2G", 2 * GB},
		{"500KB", 500 * KB},
	}

	for _, tc := range tests {
		val, err := ParseSize(tc.input, autoVal)
		if err != nil {
			t.Fatalf("unexpected error parsing %q: %v", tc.input, err)
		}
		if val != tc.expected {
			t.Errorf("input %q: expected %d bytes, got %d bytes", tc.input, tc.expected, val)
		}
	}
}

func TestMemoryLRUEviction(t *testing.T) {
	// Limit to 100 bytes
	lru := NewMemoryLRU(100)

	// Set item 1 (40 bytes)
	data1 := make([]byte, 40)
	lru.Set(&Item{Key: "key1", Data: data1, Size: 40})

	// Set item 2 (40 bytes)
	data2 := make([]byte, 40)
	lru.Set(&Item{Key: "key2", Data: data2, Size: 40})

	items, bytesCount := lru.Size()
	if items != 2 || bytesCount != 80 {
		t.Fatalf("expected 2 items and 80 bytes, got %d items and %d bytes", items, bytesCount)
	}

	// Access key1 so key2 becomes LRU
	if _, ok := lru.Get("key1"); !ok {
		t.Fatalf("key1 should exist")
	}

	// Set item 3 (40 bytes) -> should cause key2 to be evicted to keep total <= 100
	data3 := make([]byte, 40)
	lru.Set(&Item{Key: "key3", Data: data3, Size: 40})

	if _, ok := lru.Get("key2"); ok {
		t.Errorf("key2 should have been evicted")
	}
	if _, ok := lru.Get("key1"); !ok {
		t.Errorf("key1 should still exist")
	}
	if _, ok := lru.Get("key3"); !ok {
		t.Errorf("key3 should exist")
	}
}

func TestIsCacheableAsset(t *testing.T) {
	tests := []struct {
		service  string
		method   string
		path     string
		expected bool
	}{
		// Jellyfin matches
		{"jellyfin", "GET", "/Items/123/Images/Primary", true},
		{"jellyfin", "GET", "/Items/456/Images/Backdrop/0", true},
		{"jellyfin", "GET", "/UserImages/789", true},
		{"jellyfin", "GET", "/web/favicon.png", true},
		// Jellyfin streams (MUST NOT cache)
		{"jellyfin", "GET", "/Videos/123/stream.mp4", false},
		{"jellyfin", "GET", "/Videos/123/master.m3u8", false},
		{"jellyfin", "GET", "/Audio/123/stream", false},
		// Plex matches
		{"plex", "GET", "/photo/:/transcode?width=400&height=600", true},
		{"plex", "GET", "/library/metadata/10/thumb", true},
		{"plex", "GET", "/library/metadata/10/art", true},
		// Plex streams (MUST NOT cache)
		{"plex", "GET", "/video/:/transcode/universal/session.m3u8", false},
		// Immich matches
		{"immich", "GET", "/api/assets/abc-123/thumbnail", true},
		// Non-GET requests
		{"jellyfin", "POST", "/Items/123/Images/Primary", false},
	}

	for _, tc := range tests {
		actual := IsCacheableAsset(tc.service, tc.method, tc.path)
		if actual != tc.expected {
			t.Errorf("path %s %s: expected %v, got %v", tc.method, tc.path, tc.expected, actual)
		}
	}
}

func TestCalculateAutoRAMCacheSizeWithDisk(t *testing.T) {
	tests := []struct {
		ram      int64
		expected int64
	}{
		{ram: 64 * MB, expected: 8 * MB},
		{ram: 128 * MB, expected: 8 * MB},
		{ram: 256 * MB, expected: 32 * MB},
		{ram: 512 * MB, expected: 32 * MB},
		{ram: 1 * GB, expected: 64 * MB},   // 1GB SBCs get 64MB hot-buffer
		{ram: 2 * GB, expected: 64 * MB},   // 2GB SBCs get 64MB hot-buffer
		{ram: 4 * GB, expected: 128 * MB},  // 4GB+ devices get 128MB hot-buffer (~3,200 posters)
		{ram: 16 * GB, expected: 128 * MB}, // Homelabs/servers get 128MB hot-buffer (~3,200 posters)
	}

	for _, tc := range tests {
		actual := CalculateAutoRAMCacheSizeWithDisk(tc.ram)
		if actual != tc.expected {
			t.Errorf("For RAM %s with disk, expected cache size %s, got %s",
				FormatSize(tc.ram), FormatSize(tc.expected), FormatSize(actual))
		}
	}
}

func TestDualTierCache(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "meshbeacon_cache_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	c, err := NewDualTierCache(100*MB, tempDir, 500*MB, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to create dual tier cache: %v", err)
	}

	key := CacheKey("jellyfin", "/Items/1/Images/Primary", "width=300")
	testData := []byte("fake_image_payload_jpeg")
	c.Set(key, testData, "image/jpeg", "etag123")

	// 1. Immediate memory HIT
	item, ok := c.Get(key)
	if !ok {
		t.Fatalf("expected cache HIT")
	}
	if !bytes.Equal(item.Data, testData) {
		t.Errorf("data mismatch")
	}
	if item.ContentType != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %s", item.ContentType)
	}

	stats := c.Stats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
}

func TestPureDiskCache(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "meshbeacon_puredisk_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize with 0 RAM bytes (pure disk mode)
	c, err := NewDualTierCache(0, tempDir, 500*MB, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to create pure disk cache: %v", err)
	}

	key := CacheKey("plex", "/photo/:/transcode", "width=400")
	testData := []byte("fake_plex_poster_png")
	c.Set(key, testData, "image/png", "etag456")

	// Allow goroutine async disk write to settle
	time.Sleep(50 * time.Millisecond)

	// Verify RAM is completely empty
	stats := c.Stats()
	if stats.RAMBytes != 0 || stats.RAMItems != 0 {
		t.Errorf("expected 0 RAM items in pure disk mode, got %d items (%d bytes)", stats.RAMItems, stats.RAMBytes)
	}

	// Retrieve item - should hit disk
	item, ok := c.Get(key)
	if !ok {
		t.Fatalf("expected cache HIT from disk")
	}
	if !bytes.Equal(item.Data, testData) {
		t.Errorf("data mismatch: expected %q, got %q", testData, item.Data)
	}

	// Verify no promotion into RAM occurred
	statsAfter := c.Stats()
	if statsAfter.RAMBytes != 0 || statsAfter.RAMItems != 0 {
		t.Errorf("item was unexpectedly promoted into RAM: %d items (%d bytes)", statsAfter.RAMItems, statsAfter.RAMBytes)
	}
	if statsAfter.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", statsAfter.Hits)
	}
}
