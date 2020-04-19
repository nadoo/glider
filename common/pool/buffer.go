package pool

import (
	"sync"
)

var bufSizes = [...]int{
	1 << 0, 1 << 1, 1 << 2, 1 << 3, 1 << 4, 1 << 5, 1 << 6, 1 << 7, 1 << 8, 1 << 9,
	1 << 10, 2 << 10, 4 << 10, 8 << 10, 16 << 10, 32 << 10, 64 << 10,
}

var bufPools = [...]sync.Pool{
	{New: func() interface{} { return make([]byte, 1<<0) }},
	{New: func() interface{} { return make([]byte, 1<<1) }},
	{New: func() interface{} { return make([]byte, 1<<2) }},
	{New: func() interface{} { return make([]byte, 1<<3) }},
	{New: func() interface{} { return make([]byte, 1<<4) }},
	{New: func() interface{} { return make([]byte, 1<<5) }},
	{New: func() interface{} { return make([]byte, 1<<6) }},
	{New: func() interface{} { return make([]byte, 1<<7) }},
	{New: func() interface{} { return make([]byte, 1<<8) }},
	{New: func() interface{} { return make([]byte, 1<<9) }},
	{New: func() interface{} { return make([]byte, 1<<10) }},
	{New: func() interface{} { return make([]byte, 2<<10) }},
	{New: func() interface{} { return make([]byte, 4<<10) }},
	{New: func() interface{} { return make([]byte, 8<<10) }},
	{New: func() interface{} { return make([]byte, 16<<10) }},
	{New: func() interface{} { return make([]byte, 32<<10) }},
	{New: func() interface{} { return make([]byte, 64<<10) }},
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
