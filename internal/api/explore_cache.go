package api

import (
	"sync"
	"time"
)

// TTLCache is a generic cache with time-to-live expiration.
type TTLCache[K comparable, V any] struct {
	mu      sync.RWMutex
	items   map[K]*cacheItem[V]
	ttl     time.Duration
	stopCh  chan struct{}
	stopped bool
}

type cacheItem[V any] struct {
	value     V
	expiresAt time.Time
}

// NewTTLCache creates a new TTL cache with the given expiration duration.
// It starts a background goroutine that cleans up expired entries.
func NewTTLCache[K comparable, V any](ttl time.Duration) *TTLCache[K, V] {
	c := &TTLCache[K, V]{
		items:  make(map[K]*cacheItem[V]),
		ttl:    ttl,
		stopCh: make(chan struct{}),
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
func (c *TTLCache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &cacheItem[V]{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
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
