package obfs

import (
	"math/rand"

	"github.com/sun8911879/shadowsocksR/ssr"
)

type randomHead struct {
	ssr.ServerInfoForObfs
	rawTransSent     bool
	rawTransReceived bool
	hasSentHeader    bool
	dataBuffer       []byte
}

func init() {
	register("random_head", newRandomHead)
}

func newRandomHead() IObfs {
	p := &randomHead{}
	return p
}

func (r *randomHead) SetServerInfo(s *ssr.ServerInfoForObfs) {
	r.ServerInfoForObfs = *s
}

func (r *randomHead) GetServerInfo() (s *ssr.ServerInfoForObfs) {
	return &r.ServerInfoForObfs
}

func (r *randomHead) SetData(data interface{}) {

}

func (r *randomHead) GetData() interface{} {
	return nil
}

func (r *randomHead) Encode(data []byte) (encodedData []byte, err error) {
	if r.rawTransSent {
		return data, nil
	}

	dataLength := len(data)
	if r.hasSentHeader {
		if dataLength > 0 {
			d := make([]byte, len(r.dataBuffer)+dataLength)
			copy(d, r.dataBuffer)
			copy(d[len(r.dataBuffer):], data)
			r.dataBuffer = d
		} else {
			encodedData = r.dataBuffer
			r.dataBuffer = nil
			r.rawTransSent = true
		}
	} else {
		size := rand.Intn(96) + 8
		encodedData = make([]byte, size)
		rand.Read(encodedData)
		ssr.SetCRC32(encodedData, size)

		d := make([]byte, dataLength)
		copy(d, data)
		r.dataBuffer = d
	}
	r.hasSentHeader = true
	return
}

func (r *randomHead) Decode(data []byte) ([]byte, uint64, error) {
	if r.rawTransReceived {
		return data, 0, nil
	}
	r.rawTransReceived = true
	return data, 0, nil
}
