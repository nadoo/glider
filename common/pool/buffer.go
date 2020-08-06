package pool

import (
	"sync"
)

const num = 17 // number of pools, pool size range: 1<<0 ~ 1<<16 bytes. (1Byte~64KBytes)

var (
	pools [num]sync.Pool
	sizes [num]int
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
	for i := 0; i < num; i++ {
		if size <= sizes[i] {
			return pools[i].Get().([]byte)[:size]
		}
	}
	return make([]byte, size)
}

// PutBuffer puts a buffer into pool.
func PutBuffer(buf []byte) {
	c := cap(buf)
	for i := 0; i < num; i++ {
		if c == sizes[i] {
			pools[i].Put(buf)
			return
		}
	}
}
