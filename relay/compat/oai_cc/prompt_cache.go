package oai_cc

import (
	"container/list"
	"sync"
	"time"
)

type PromptCacheStats struct {
	MaxEntries   int
	Size         int
	Reads        int
	Hits         int
	Misses       int
	Creates      int
	Evictions    int
	Expires      int
	LastResetAt  time.Time
	LastResetAtS string
}

type promptCacheEntry struct {
	key          string
	value        int
	expiresAt    time.Time
	createdAt    time.Time
	lastAccessAt time.Time
}

type PromptCache struct {
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

func NewPromptCache(maxEntries int) *PromptCache {
	if maxEntries < 0 {
		maxEntries = 0
	}
	return &PromptCache{
		MaxEntries: maxEntries,
		ll:        list.New(),
		items:     make(map[string]*list.Element),
		lastReset: time.Now(),
	}
}

func (c *PromptCache) SetMaxEntries(maxEntries int) {
	if c == nil {
		return
	}
	if maxEntries < 0 {
		maxEntries = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.MaxEntries = maxEntries
	for c.MaxEntries > 0 && len(c.items) > c.MaxEntries {
		front := c.ll.Front()
		if front == nil {
			break
		}
		ent, _ := front.Value.(*promptCacheEntry)
		if ent != nil {
			delete(c.items, ent.key)
		}
		c.ll.Remove(front)
		c.evictions++
	}
}

func (c *PromptCache) GetStats() PromptCacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	return PromptCacheStats{
		MaxEntries:   c.MaxEntries,
		Size:         len(c.items),
		Reads:        c.reads,
		Hits:         c.hits,
		Misses:       c.misses,
		Creates:      c.creates,
		Evictions:    c.evictions,
		Expires:      c.expires,
		LastResetAt:  c.lastReset,
		LastResetAtS: c.lastReset.UTC().Format(time.RFC3339Nano),
	}
}

func (c *PromptCache) Clear() {
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

func (c *PromptCache) isExpired(ent *promptCacheEntry, now time.Time) bool {
	if ent == nil {
		return true
	}
	if ent.expiresAt.IsZero() {
		return false
	}
	return now.After(ent.expiresAt)
}

func (c *PromptCache) Get(key string) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.reads++

	elem := c.items[key]
	if elem == nil {
		c.misses++
		return 0, false
	}
	ent, ok := elem.Value.(*promptCacheEntry)
	if !ok || ent == nil {
		delete(c.items, key)
		c.ll.Remove(elem)
		c.misses++
		return 0, false
	}

	now := time.Now()
	if c.isExpired(ent, now) {
		delete(c.items, key)
		c.ll.Remove(elem)
		c.misses++
		c.expires++
		return 0, false
	}

	c.hits++
	ent.lastAccessAt = now
	c.ll.MoveToBack(elem)
	return ent.value, true
}

func (c *PromptCache) Set(key string, value int, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	elem := c.items[key]
	if elem != nil {
		if ent, ok := elem.Value.(*promptCacheEntry); ok && ent != nil {
			ent.value = value
			ent.lastAccessAt = now
			if ttl > 0 {
				ent.expiresAt = now.Add(ttl)
			} else {
				ent.expiresAt = time.Time{}
			}
			c.ll.MoveToBack(elem)
		} else {
			delete(c.items, key)
			c.ll.Remove(elem)
			elem = nil
		}
	}

	if elem == nil {
		ent := &promptCacheEntry{
			key:          key,
			value:        value,
			createdAt:    now,
			lastAccessAt: now,
		}
		if ttl > 0 {
			ent.expiresAt = now.Add(ttl)
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
		ent, _ := front.Value.(*promptCacheEntry)
		if ent != nil {
			delete(c.items, ent.key)
		}
		c.ll.Remove(front)
		c.evictions++
	}
}
