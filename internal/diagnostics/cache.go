package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type cacheEntry struct {
	Quality IPQuality
	Expires time.Time
}

type Checker struct {
	cachePath string
	mu        sync.Mutex
	entries   map[string]cacheEntry
}

func New(dataDir string) *Checker {
	return &Checker{cachePath: filepath.Join(dataDir, "ip-quality-cache.json")}
}

func (c *Checker) cached(ip string) (IPQuality, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		var entries map[string]cacheEntry
		data, err := os.ReadFile(c.cachePath)
		if err != nil || json.Unmarshal(data, &entries) != nil || entries == nil {
			entries = make(map[string]cacheEntry)
		}
		c.entries = entries
	}
	entry := c.entries[ip]
	return entry.Quality, time.Now().Before(entry.Expires) && entry.Quality.Risk.Score != nil
}

func (c *Checker) store(ip string, quality IPQuality) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, entry := range c.entries {
		if !now.Before(entry.Expires) {
			delete(c.entries, key)
		}
	}
	c.entries[ip] = cacheEntry{Quality: quality, Expires: now.Add(72 * time.Hour)}
	// A read-only or full data directory still permits in-memory caching.
	data, err := json.Marshal(c.entries)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.cachePath), ".ip-quality-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(data)
	if err := tmp.Close(); err == nil && writeErr == nil {
		_ = os.Rename(tmp.Name(), c.cachePath)
	}
}
