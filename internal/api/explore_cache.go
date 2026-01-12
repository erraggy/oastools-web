package api

import (
	"sync"
	"time"
)

// TTLCache is a generic cache with time-to-live expiration and max entries limit.
type TTLCache[K comparable, V any] struct {
	mu         sync.RWMutex
	items      map[K]*cacheItem[V]
	ttl        time.Duration
	maxEntries int
	stopCh     chan struct{}
	stopped    bool
}

type cacheItem[V any] struct {
	value     V
	expiresAt time.Time
	addedAt   time.Time
}

// NewTTLCache creates a new TTL cache with the given expiration duration and max entries.
// It starts a background goroutine that cleans up expired entries.
// If maxEntries is 0, no limit is enforced.
func NewTTLCache[K comparable, V any](ttl time.Duration, maxEntries int) *TTLCache[K, V] {
	c := &TTLCache[K, V]{
		items:      make(map[K]*cacheItem[V]),
		ttl:        ttl,
		maxEntries: maxEntries,
		stopCh:     make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

// Get retrieves a value from the cache.
// Returns the value and true if found and not expired, zero value and false otherwise.
func (c *TTLCache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}

	if time.Now().After(item.expiresAt) {
		var zero V
		return zero, false
	}

	return item.value, true
}

// Set stores a value in the cache with the configured TTL.
// If maxEntries is exceeded, the oldest entry is evicted.
func (c *TTLCache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// If key already exists, just update it
	if _, exists := c.items[key]; !exists {
		// Evict oldest entry if at capacity
		if c.maxEntries > 0 && len(c.items) >= c.maxEntries {
			c.evictOldest()
		}
	}

	c.items[key] = &cacheItem[V]{
		value:     value,
		expiresAt: now.Add(c.ttl),
		addedAt:   now,
	}
}

// evictOldest removes the oldest entry from the cache.
// Must be called with lock held.
func (c *TTLCache[K, V]) evictOldest() {
	var oldestKey K
	var oldestTime time.Time
	first := true

	for key, item := range c.items {
		if first || item.addedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = item.addedAt
			first = false
		}
	}

	if !first {
		delete(c.items, oldestKey)
	}
}

// Delete removes a value from the cache.
func (c *TTLCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Stop stops the cleanup goroutine.
func (c *TTLCache[K, V]) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		close(c.stopCh)
		c.stopped = true
	}
}

// cleanupLoop removes expired entries every 30 seconds.
func (c *TTLCache[K, V]) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

func (c *TTLCache[K, V]) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, item := range c.items {
		if now.After(item.expiresAt) {
			delete(c.items, key)
		}
	}
}
