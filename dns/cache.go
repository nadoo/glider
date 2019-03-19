package dns

import (
	"sync"
	"time"
)

// LongTTL is 50 years duration in seconds, used for none-expired items.
const LongTTL = 50 * 365 * 24 * 3600

type item struct {
	value  []byte
	expire time.Time
}

// Cache is the struct of cache.
type Cache struct {
	m map[string]*item
	l sync.RWMutex
}

// NewCache returns a new cache.
func NewCache() (c *Cache) {
	c = &Cache{m: make(map[string]*item)}
	go func() {
		for now := range time.Tick(time.Second) {
			c.l.Lock()
			for k, v := range c.m {
				if now.After(v.expire) {
					delete(c.m, k)
				}
			}
			c.l.Unlock()
		}
	}()
	return
}

// Len returns the length of cache.
func (c *Cache) Len() int {
	return len(c.m)
}

// Put an item into cache, invalid after ttl seconds.
func (c *Cache) Put(k string, v []byte, ttl int) {
	if len(v) != 0 {
		c.l.Lock()
		it, ok := c.m[k]
		if !ok {
			it = &item{value: v}
			c.m[k] = it
		}
		it.expire = time.Now().Add(time.Duration(ttl) * time.Second)
		c.l.Unlock()
	}
}

// Get an item from cache.
func (c *Cache) Get(k string) (v []byte) {
	c.l.RLock()
	if it, ok := c.m[k]; ok {
		v = it.value
	}
	c.l.RUnlock()
	return
}
