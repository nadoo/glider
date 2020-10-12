package dns

import (
	"sync"
	"time"
)

// LruCache is the struct of LruCache.
type LruCache struct {
	mu    sync.Mutex
	size  int
	head  *item
	tail  *item
	cache map[string]*item
	store map[string][]byte
}

// item is the struct of cache item.
type item struct {
	key  string
	val  []byte
	exp  int64
	prev *item
	next *item
}

// NewLruCache returns a new LruCache.
func NewLruCache(size int) *LruCache {
	// init 2 items here, it doesn't matter cuz they will be deleted when the cache is full
	head, tail := &item{key: "head"}, &item{key: "tail"}
	head.next, tail.prev = tail, head
	c := &LruCache{
		size:  size,
		head:  head,
		tail:  tail,
		cache: make(map[string]*item, size),
		store: make(map[string][]byte),
	}
	c.cache[head.key], c.cache[tail.key] = head, tail
	return c
}

// Get gets an item from cache.
func (c *LruCache) Get(k string) (v []byte, expired bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if v, ok := c.store[k]; ok {
		return v, false
	}

	if it, ok := c.cache[k]; ok {
		v = it.val
		if it.exp < time.Now().Unix() {
			expired = true
		}
		c.moveToHead(it)
	}
	return
}

// Set sets an item with key, value, and ttl(seconds).
// if the ttl is zero, this item will be set and never be deleted.
// if the key exists, update it with value and exp and move it to head.
// if the key does not exist, put a new item to the cache's head.
// finally, remove the tail if the cache is full.
func (c *LruCache) Set(k string, v []byte, ttl int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ttl == 0 {
		c.store[k] = v
		return
	}

	exp := time.Now().Add(time.Second * time.Duration(ttl)).Unix()
	if it, ok := c.cache[k]; ok {
		it.val = v
		it.exp = exp
		c.moveToHead(it)
		return
	}

	c.putToHead(k, v, exp)

	// NOTE: the cache size will always >= 2,
	// but it doesn't matter in our environment.
	if len(c.cache) > c.size {
		c.removeTail()
	}
}

// putToHead puts a new item to cache's head.
func (c *LruCache) putToHead(k string, v []byte, exp int64) {
	it := &item{key: k, val: v, exp: exp, prev: nil, next: c.head}
	it.prev = nil
	it.next = c.head
	c.head.prev = it
	c.head = it

	c.cache[k] = it
}

// moveToHead moves an existing item to cache's head.
func (c *LruCache) moveToHead(it *item) {
	if it != c.head {
		if c.tail == it {
			c.tail = it.prev
			c.tail.next = nil
		} else {
			it.prev.next = it.next
			it.next.prev = it.prev
		}
		it.prev = nil
		it.next = c.head
		c.head.prev = it
		c.head = it
	}
}

// removeTail removes the tail from cache.
func (c *LruCache) removeTail() {
	delete(c.cache, c.tail.key)

	c.tail.prev.next = nil
	c.tail = c.tail.prev
}
