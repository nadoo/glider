package pool

import (
	"bytes"
	"sync"
)

var writeBufPool = sync.Pool{
	New: func() interface{} { return &bytes.Buffer{} },
}

func GetWriteBuffer() *bytes.Buffer {
	return writeBufPool.Get().(*bytes.Buffer)
}

func PutWriteBuffer(buf *bytes.Buffer) {
	if buf.Cap() > 64<<10 {
		return
	}

	buf.Reset()
	writeBufPool.Put(buf)
}
