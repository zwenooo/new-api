package oai_cc

import (
	"container/list"
	"sync"
	"time"
)

type SessionCacheStats struct {
	TTL        time.Duration
	MaxEntries int
	Size       int
	Reads      int
	Hits       int
	Misses     int
	Creates    int
	Evictions  int
	Expires    int
	LastReset  time.Time
}

type sessionCacheEntry[T any] struct {
	key          string
	value        T
	createdAt    time.Time
	lastAccessAt time.Time
}

type SessionCache[T any] struct {
	TTL        time.Duration
	MaxEntries int

	mu    sync.Mutex
	ll    *list.List
	items map[string]*list.Element

	reads     int
	hits      int
	misses    int
	creates   int
	evictions int
	expires   int
	lastReset time.Time
}

func NewSessionCache[T any](ttl time.Duration, maxEntries int) *SessionCache[T] {
	if maxEntries < 0 {
		maxEntries = 0
	}
	if ttl < 0 {
		ttl = 0
	}
	return &SessionCache[T]{
		TTL:       ttl,
		MaxEntries: maxEntries,
		ll:        list.New(),
		items:     make(map[string]*list.Element),
		lastReset: time.Now(),
	}
}

func (c *SessionCache[T]) UpdateConfig(ttl time.Duration, maxEntries int) {
	if c == nil {
		return
	}
	if ttl < 0 {
		ttl = 0
	}
	if maxEntries < 0 {
		maxEntries = 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.TTL = ttl
	c.MaxEntries = maxEntries

	for c.MaxEntries > 0 && len(c.items) > c.MaxEntries {
		front := c.ll.Front()
		if front == nil {
			break
		}
		ent, _ := front.Value.(*sessionCacheEntry[T])
		if ent != nil {
			delete(c.items, ent.key)
		}
		c.ll.Remove(front)
		c.evictions++
	}
}

func (c *SessionCache[T]) GetStats() SessionCacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	return SessionCacheStats{
		TTL:        c.TTL,
		MaxEntries: c.MaxEntries,
		Size:       len(c.items),
		Reads:      c.reads,
		Hits:       c.hits,
		Misses:     c.misses,
		Creates:    c.creates,
		Evictions:  c.evictions,
		Expires:    c.expires,
		LastReset:  c.lastReset,
	}
}

func (c *SessionCache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ll.Init()
	c.items = make(map[string]*list.Element)
	c.reads = 0
	c.hits = 0
	c.misses = 0
	c.creates = 0
	c.evictions = 0
	c.expires = 0
	c.lastReset = time.Now()
}

func (c *SessionCache[T]) isExpired(ent *sessionCacheEntry[T], now time.Time) bool {
	if ent == nil {
		return true
	}
	if c.TTL <= 0 {
		return false
	}
	return now.Sub(ent.lastAccessAt) > c.TTL
}

func (c *SessionCache[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var zero T
	c.reads++

	elem := c.items[key]
	if elem == nil {
		c.misses++
		return zero, false
	}
	ent, ok := elem.Value.(*sessionCacheEntry[T])
	if !ok || ent == nil {
		delete(c.items, key)
		c.ll.Remove(elem)
		c.misses++
		return zero, false
	}

	now := time.Now()
	if c.isExpired(ent, now) {
		delete(c.items, key)
		c.ll.Remove(elem)
		c.misses++
		c.expires++
		return zero, false
	}

	c.hits++
	ent.lastAccessAt = now
	c.ll.MoveToBack(elem)
	return ent.value, true
}

func (c *SessionCache[T]) Set(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	elem := c.items[key]
	if elem != nil {
		if ent, ok := elem.Value.(*sessionCacheEntry[T]); ok && ent != nil {
			ent.value = value
			ent.lastAccessAt = now
			c.ll.MoveToBack(elem)
		} else {
			delete(c.items, key)
			c.ll.Remove(elem)
			elem = nil
		}
	}

	if elem == nil {
		ent := &sessionCacheEntry[T]{
			key:          key,
			value:        value,
			createdAt:    now,
			lastAccessAt: now,
		}
		elem = c.ll.PushBack(ent)
		c.items[key] = elem
		c.creates++
	}

	for c.MaxEntries > 0 && len(c.items) > c.MaxEntries {
		front := c.ll.Front()
		if front == nil {
			break
		}
		ent, _ := front.Value.(*sessionCacheEntry[T])
		if ent != nil {
			delete(c.items, ent.key)
		}
		c.ll.Remove(front)
		c.evictions++
	}
}

func (c *SessionCache[T]) GetOrCreate(key string, createFn func() T) (value T, hit bool) {
	if c == nil {
		var zero T
		return zero, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.reads++

	elem := c.items[key]
	if elem != nil {
		ent, ok := elem.Value.(*sessionCacheEntry[T])
		if ok && ent != nil && !c.isExpired(ent, time.Now()) {
			c.hits++
			ent.lastAccessAt = time.Now()
			c.ll.MoveToBack(elem)
			return ent.value, true
		}

		// Stale/bad entry.
		delete(c.items, key)
		c.ll.Remove(elem)
		c.misses++
		if ent != nil {
			c.expires++
		}
	}

	c.misses++

	value = createFn()
	now := time.Now()
	ent := &sessionCacheEntry[T]{
		key:          key,
		value:        value,
		createdAt:    now,
		lastAccessAt: now,
	}
	elem = c.ll.PushBack(ent)
	c.items[key] = elem
	c.creates++

	for c.MaxEntries > 0 && len(c.items) > c.MaxEntries {
		front := c.ll.Front()
		if front == nil {
			break
		}
		oldEnt, _ := front.Value.(*sessionCacheEntry[T])
		if oldEnt != nil {
			delete(c.items, oldEnt.key)
		}
		c.ll.Remove(front)
		c.evictions++
	}

	return value, false
}
