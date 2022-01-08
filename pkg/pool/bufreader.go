package pool

import (
	"bufio"
	"io"
	"sync"
)

var bufReaderPool sync.Pool

// GetBufReader returns a *bufio.Reader from pool.
func GetBufReader(r io.Reader) *bufio.Reader {
	if v := bufReaderPool.Get(); v != nil {
		br := v.(*bufio.Reader)
		br.Reset(r)
		return br
	}
	return bufio.NewReader(r)
}

// PutBufReader puts a *bufio.Reader into pool.
func PutBufReader(br *bufio.Reader) {
	bufReaderPool.Put(br)
}
