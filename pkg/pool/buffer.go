package pool

import (
	"math/bits"
	"sync"
	"unsafe"
)

const (
	// number of pools.
	num     = 17
	maxsize = 1 << (num - 1)
)

var (
	sizes [num]int
	pools [num]sync.Pool
)

func init() {
	for i := range num {
		size := 1 << i
		sizes[i] = size
		pools[i].New = func() any {
			buf := make([]byte, size)
			return unsafe.SliceData(buf)
		}
	}
}

// GetBuffer gets a buffer from pool, size should in range: [1, 65536],
// otherwise, this function will call make([]byte, size) directly.
func GetBuffer(size int) []byte {
	if size >= 1 && size <= maxsize {
		i := bits.Len32(uint32(size - 1))
		if p := pools[i].Get().(*byte); p != nil {
			return unsafe.Slice(p, 1<<i)[:size]
		}
	}
	return make([]byte, size)
}

// PutBuffer puts a buffer into pool.
func PutBuffer(buf []byte) {
	if size := cap(buf); size >= 1 && size <= maxsize {
		i := bits.Len32(uint32(size - 1))
		if sizes[i] == size {
			pools[i].Put(unsafe.SliceData(buf))
		}
	}
}
