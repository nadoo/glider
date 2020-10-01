package dns

import (
	"sync"
	"time"

	"github.com/nadoo/glider/pool"
)

// LongTTL is 50 years duration in seconds, used for none-expired items.
const LongTTL = 50 * 365 * 24 * 3600

type item struct {
	value  []byte
	expire time.Time
}

// Cache is the struct of cache.
type Cache struct {
	store     map[string]*item
	mutex     sync.RWMutex
	storeCopy bool
}

// NewCache returns a new cache.
func NewCache(storeCopy bool) (c *Cache) {
	c = &Cache{store: make(map[string]*item), storeCopy: storeCopy}
	go func() {
		for now := range time.Tick(time.Second) {
			c.mutex.Lock()
			for k, v := range c.store {
				if now.After(v.expire) {
					delete(c.store, k)
					if storeCopy {
						pool.PutBuffer(v.value)
					}
				}
			}
			c.mutex.Unlock()
		}
	}()
	return
}

// Len returns the length of cache.
func (c *Cache) Len() int {
	return len(c.store)
}

// Put an item into cache, invalid after ttl seconds.
func (c *Cache) Put(k string, v []byte, ttl int) {
	if len(v) != 0 {
		c.mutex.Lock()
		it, ok := c.store[k]
		if !ok {
			if c.storeCopy {
				it = &item{value: valCopy(v)}
			} else {
				it = &item{value: v}
			}
			c.store[k] = it
		}
		it.expire = time.Now().Add(time.Duration(ttl) * time.Second)
		c.mutex.Unlock()
	}
}

// Get gets an item from cache(do not modify it).
func (c *Cache) Get(k string) (v []byte) {
	c.mutex.RLock()
	if it, ok := c.store[k]; ok {
		v = it.value
	}
	c.mutex.RUnlock()
	return
}

// GetCopy gets an item from cache and returns it's copy(so you can modify it).
func (c *Cache) GetCopy(k string) []byte {
	return valCopy(c.Get(k))
}

func valCopy(v []byte) (b []byte) {
	if v != nil {
		b = pool.GetBuffer(len(v))
		copy(b, v)
	}
	return
}
