package api

import (
	"sync"
	"testing"
	"time"
)

func TestTTLCache_SetGet(t *testing.T) {
	cache := NewTTLCache[string, int](1 * time.Minute)
	defer cache.Stop()

	cache.Set("key1", 100)
	cache.Set("key2", 200)

	val, ok := cache.Get("key1")
	if !ok || val != 100 {
		t.Errorf("Get(key1) = %d, %v; want 100, true", val, ok)
	}

	val, ok = cache.Get("key2")
	if !ok || val != 200 {
		t.Errorf("Get(key2) = %d, %v; want 200, true", val, ok)
	}

	_, ok = cache.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestTTLCache_Expiration(t *testing.T) {
	cache := NewTTLCache[string, string](50 * time.Millisecond)
	defer cache.Stop()

	cache.Set("key", "value")

	val, ok := cache.Get("key")
	if !ok || val != "value" {
		t.Errorf("Get(key) = %q, %v; want value, true", val, ok)
	}

	time.Sleep(100 * time.Millisecond)

	_, ok = cache.Get("key")
	if ok {
		t.Error("Get(key) should return false after expiration")
	}
}

func TestTTLCache_Delete(t *testing.T) {
	cache := NewTTLCache[string, int](1 * time.Minute)
	defer cache.Stop()

	cache.Set("key", 42)
	cache.Delete("key")

	_, ok := cache.Get("key")
	if ok {
		t.Error("Get(key) should return false after delete")
	}
}

func TestTTLCache_Concurrent(t *testing.T) {
	cache := NewTTLCache[int, int](1 * time.Minute)
	defer cache.Stop()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cache.Set(n, n*2)
			cache.Get(n)
		}(i)
	}
	wg.Wait()

	// Verify some values
	for i := range 100 {
		val, ok := cache.Get(i)
		if !ok {
			t.Errorf("Get(%d) should return true", i)
		}
		if val != i*2 {
			t.Errorf("Get(%d) = %d; want %d", i, val, i*2)
		}
	}
}
