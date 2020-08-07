package pool

import (
	"sort"
	"sync"
)

// number of pools.
// pool sizes: 1<<0 ~ 1<<16 bytes, (1Byte~64KBytes).
const num = 17

var (
	sizes [num]int
	pools [num]sync.Pool
)

func init() {
	// use range here to get a copy of index(different k) in each loop.
	for k := range pools {
		sizes[k] = 1 << k
		pools[k].New = func() interface{} {
			return make([]byte, sizes[k])
		}
	}
}

// GetBuffer gets a buffer from pool.
func GetBuffer(size int) []byte {
	i := sort.Search(num, func(i int) bool { return sizes[i] >= size })
	if i < num {
		return pools[i].Get().([]byte)[:size]
	}
	return make([]byte, size)
}

// PutBuffer puts a buffer into pool.
func PutBuffer(buf []byte) {
	size := cap(buf)
	i := sort.Search(num, func(i int) bool { return sizes[i] >= size })
	if i < num && sizes[i] == size {
		pools[i].Put(buf)
	}
}
