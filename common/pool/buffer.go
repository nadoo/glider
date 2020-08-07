package pool

import (
	"math/bits"
	"sync"
)

const (
	// number of pools.
	num     = 17 // pool sizes: 1<<0 ~ 1<<16 bytes, (1Byte~64KBytes).
	maxsize = 1 << (num - 1)
)

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

// GetBuffer gets a buffer from pool, size should in range: [1, 65536], otherwise, this function will call make([]byte, size) directly.
func GetBuffer(size int) []byte {
	if size >= 1 && size < maxsize {
		i := bits.Len32(uint32(size)) - 1
		if sizes[i] < size {
			i += 1
		}
		return pools[i].Get().([]byte)[:size]
	}
	return make([]byte, size)
}

// PutBuffer puts a buffer into pool.
func PutBuffer(buf []byte) {
	size := cap(buf)
	i := bits.Len32(uint32(size)) - 1
	if i < num && sizes[i] == size {
		pools[i].Put(buf)
	}
}
