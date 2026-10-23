package cache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Item represents a cached binary artwork asset.
type Item struct {
	Key         string    `json:"key"`
	ContentType string    `json:"content_type"`
	ETag        string    `json:"etag"`
	ExpiresAt   time.Time `json:"expires_at"`
	Size        int64     `json:"size"`
	Data        []byte    `json:"-"`
}

// Stats provides cache performance and capacity metrics.
type Stats struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	RAMItems  int   `json:"ram_items"`
	RAMBytes  int64 `json:"ram_bytes"`
	DiskItems int   `json:"disk_items"`
	DiskBytes int64 `json:"disk_bytes"`
}

// MemoryLRU provides an in-memory thread-safe LRU cache.
type MemoryLRU struct {
	mu       sync.RWMutex
	maxBytes int64
	curBytes int64
	items    map[string]*list.Element
	evict    *list.List
}

// NewMemoryLRU creates a memory cache capped at maxBytes.
func NewMemoryLRU(maxBytes int64) *MemoryLRU {
	return &MemoryLRU{
		maxBytes: maxBytes,
		items:    make(map[string]*list.Element),
		evict:    list.New(),
	}
}

func (m *MemoryLRU) Get(key string) (*Item, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	el, ok := m.items[key]
	if !ok {
		return nil, false
	}

	item := el.Value.(*Item)
	if !item.ExpiresAt.IsZero() && time.Now().After(item.ExpiresAt) {
		m.removeElement(el)
		return nil, false
	}

	m.evict.MoveToFront(el)
	return item, true
}

func (m *MemoryLRU) Set(item *Item) {
	if m.maxBytes <= 0 || item.Size > m.maxBytes {
		return // item too large or cache disabled
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// If existing key, update and move to front
	if el, ok := m.items[item.Key]; ok {
		oldItem := el.Value.(*Item)
		m.curBytes -= oldItem.Size
		el.Value = item
		m.curBytes += item.Size
		m.evict.MoveToFront(el)
		m.evictOverCapacity()
		return
	}

	// Add new entry
	el := m.evict.PushFront(item)
	m.items[item.Key] = el
	m.curBytes += item.Size

	m.evictOverCapacity()
}

func (m *MemoryLRU) evictOverCapacity() {
	for m.curBytes > m.maxBytes && m.evict.Len() > 0 {
		back := m.evict.Back()
		if back != nil {
			m.removeElement(back)
		}
	}
}

func (m *MemoryLRU) removeElement(el *list.Element) {
	item := el.Value.(*Item)
	m.curBytes -= item.Size
	m.evict.Remove(el)
	delete(m.items, item.Key)
}

func (m *MemoryLRU) Size() (int, int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items), m.curBytes
}

// DiskTier provides persistent secondary file storage for assets.
type DiskTier struct {
	dir      string
	maxBytes int64
	curBytes int64
	mu       sync.Mutex
}

// NewDiskTier initializes disk caching if dir is non-empty.
func NewDiskTier(dir string, maxBytes int64) (*DiskTier, error) {
	if dir == "" || maxBytes <= 0 {
		return nil, nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory %s: %w", dir, err)
	}

	dt := &DiskTier{
		dir:      dir,
		maxBytes: maxBytes,
	}
	dt.scanCurrentBytes()
	return dt, nil
}

func (d *DiskTier) scanCurrentBytes() {
	var total int64
	_ = filepath.Walk(d.dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	d.curBytes = total
}

func (d *DiskTier) filePath(key string) string {
	h := sha256.Sum256([]byte(key))
	hashStr := hex.EncodeToString(h[:])
	// 2-level directory tree to prevent single directory bloat
	return filepath.Join(d.dir, hashStr[:2], hashStr[2:4], hashStr)
}

func (d *DiskTier) Get(key string) (*Item, bool) {
	fp := d.filePath(key)
	data, err := os.ReadFile(fp)
	if err != nil {
		return nil, false
	}

	metaFile := fp + ".meta"
	metaBytes, err := os.ReadFile(metaFile)
	if err != nil {
		return nil, false
	}

	var item Item
	if err := json.Unmarshal(metaBytes, &item); err != nil {
		return nil, false
	}

	if !item.ExpiresAt.IsZero() && time.Now().After(item.ExpiresAt) {
		_ = os.Remove(fp)
		_ = os.Remove(metaFile)
		return nil, false
	}

	item.Data = data
	return &item, true
}

func (d *DiskTier) Set(item *Item) {
	fp := d.filePath(keyFor(item.Key))
	dir := filepath.Dir(fp)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	meta := Item{
		Key:         item.Key,
		ContentType: item.ContentType,
		ETag:        item.ETag,
		ExpiresAt:   item.ExpiresAt,
		Size:        item.Size,
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if err := os.WriteFile(fp, item.Data, 0644); err == nil {
		_ = os.WriteFile(fp+".meta", metaBytes, 0644)
		d.curBytes += item.Size
	}
}

func (d *DiskTier) Size() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.curBytes
}

func keyFor(k string) string {
	return k
}

// DualTierCache coordinates RAM LRU and optional secondary Disk storage.
type DualTierCache struct {
	ram    *MemoryLRU
	disk   *DiskTier
	ttl    time.Duration
	hits   int64
	misses int64
}

// NewDualTierCache creates a configured dual-tier asset cache.
func NewDualTierCache(ramBytes int64, diskDir string, diskBytes int64, ttl time.Duration) (*DualTierCache, error) {
	var disk *DiskTier
	if diskDir != "" && diskBytes > 0 {
		var err error
		disk, err = NewDiskTier(diskDir, diskBytes)
		if err != nil {
			log.Printf("[Cache Warning] Could not initialize disk cache: %v. Running in-memory only.", err)
		}
	}

	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour // default 30 days
	}

	return &DualTierCache{
		ram:  NewMemoryLRU(ramBytes),
		disk: disk,
		ttl:  ttl,
	}, nil
}

// CacheKey builds a normalized unique cache key from service and URL path+query.
func CacheKey(service, path, rawQuery string) string {
	if rawQuery != "" {
		return service + ":" + path + "?" + rawQuery
	}
	return service + ":" + path
}

// Get queries RAM first, then Disk. If found on Disk, it is promoted to RAM (if RAM cache is active).
func (c *DualTierCache) Get(key string) (*Item, bool) {
	// 1. Check L1: Memory LRU (if enabled)
	if c.ram != nil && c.ram.maxBytes > 0 {
		if item, ok := c.ram.Get(key); ok {
			atomic.AddInt64(&c.hits, 1)
			return item, true
		}
	}

	// 2. Check L2: Disk Tier (if enabled)
	if c.disk != nil {
		if item, ok := c.disk.Get(key); ok {
			if c.ram != nil && c.ram.maxBytes > 0 {
				c.ram.Set(item) // Promote back into RAM
			}
			atomic.AddInt64(&c.hits, 1)
			return item, true
		}
	}

	atomic.AddInt64(&c.misses, 1)
	return nil, false
}

// Set stores an asset in RAM (if enabled) and Disk (if configured).
func (c *DualTierCache) Set(key string, data []byte, contentType, etag string) {
	item := &Item{
		Key:         key,
		Data:        data,
		ContentType: contentType,
		ETag:        etag,
		Size:        int64(len(data)),
		ExpiresAt:   time.Now().Add(c.ttl),
	}

	if c.ram != nil && c.ram.maxBytes > 0 {
		c.ram.Set(item)
	}

	if c.disk != nil {
		go c.disk.Set(item)
	}
}

// Stats returns hit counts, RAM usage, and disk metrics.
func (c *DualTierCache) Stats() Stats {
	items, bytes := c.ram.Size()
	diskBytes := int64(0)
	if c.disk != nil {
		diskBytes = c.disk.Size()
	}

	return Stats{
		Hits:      atomic.LoadInt64(&c.hits),
		Misses:    atomic.LoadInt64(&c.misses),
		RAMItems:  items,
		RAMBytes:  bytes,
		DiskBytes: diskBytes,
	}
}
