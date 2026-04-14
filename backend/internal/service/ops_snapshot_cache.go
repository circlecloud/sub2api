package service

import (
	"sync"
	"time"
)

type opsSnapshotCacheEntry[V any] struct {
	value     V
	expiresAt time.Time
}

type opsSnapshotMapCache[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K]opsSnapshotCacheEntry[V]
}

type opsSnapshotValueCache[V any] struct {
	mu    sync.RWMutex
	entry *opsSnapshotCacheEntry[V]
}

func opsSnapshotEntryExpired(now, expiresAt time.Time) bool {
	return now.After(expiresAt)
}

func (c *opsSnapshotMapCache[K, V]) get(key K, now time.Time) (V, bool) {
	var zero V
	if c == nil {
		return zero, false
	}

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return zero, false
	}
	if !opsSnapshotEntryExpired(now, entry.expiresAt) {
		return entry.value, true
	}

	c.mu.Lock()
	if current, ok := c.entries[key]; ok && opsSnapshotEntryExpired(now, current.expiresAt) {
		delete(c.entries, key)
	}
	c.mu.Unlock()
	return zero, false
}

func (c *opsSnapshotMapCache[K, V]) set(key K, value V, ttl time.Duration, now time.Time) V {
	if c == nil {
		return value
	}

	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[K]opsSnapshotCacheEntry[V])
	}
	c.entries[key] = opsSnapshotCacheEntry[V]{value: value, expiresAt: now.Add(ttl)}
	c.mu.Unlock()
	return value
}

func (c *opsSnapshotMapCache[K, V]) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

func (c *opsSnapshotValueCache[V]) get(now time.Time) (V, bool) {
	var zero V
	if c == nil {
		return zero, false
	}

	c.mu.RLock()
	entry := c.entry
	c.mu.RUnlock()
	if entry == nil {
		return zero, false
	}
	if !opsSnapshotEntryExpired(now, entry.expiresAt) {
		return entry.value, true
	}

	c.mu.Lock()
	if current := c.entry; current != nil && opsSnapshotEntryExpired(now, current.expiresAt) {
		c.entry = nil
	}
	c.mu.Unlock()
	return zero, false
}

func (c *opsSnapshotValueCache[V]) set(value V, ttl time.Duration, now time.Time) V {
	if c == nil {
		return value
	}

	c.mu.Lock()
	c.entry = &opsSnapshotCacheEntry[V]{value: value, expiresAt: now.Add(ttl)}
	c.mu.Unlock()
	return value
}

//nolint:unused // 预留给后续快照缓存显式清理路径
func (c *opsSnapshotValueCache[V]) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entry = nil
	c.mu.Unlock()
}
