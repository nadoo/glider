package pool

import (
	"sync"
)

var bufSizes = []int{
	1 << 0, 1 << 1, 1 << 2, 1 << 3, 1 << 4, 1 << 5, 1 << 6, 1 << 7, 1 << 8, 1 << 9,
	1 << 10, 2 << 10, 4 << 10, 8 << 10, 16 << 10, 32 << 10, 64 << 10,
}

var bufPools = initPools(bufSizes)

func initPools(sizes []int) []sync.Pool {
	pools := make([]sync.Pool, len(sizes))
	for k := range pools {
		pools[k].New = func() interface{} {
			return make([]byte, sizes[k])
		}
	}
	return pools
}

func GetBuffer(size int) []byte {
	i := 0
	for ; i < len(bufSizes)-1; i++ {
		if size <= bufSizes[i] {
			break
		}
	}
	return bufPools[i].Get().([]byte)[:size]
}

func PutBuffer(p []byte) {
	l := len(p)
	for i, n := range bufSizes {
		if l == n {
			bufPools[i].Put(p)
			return
		}
	}
}
