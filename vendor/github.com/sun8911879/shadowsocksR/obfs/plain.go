package obfs

import (
	"github.com/sun8911879/shadowsocksR/ssr"
)

func init() {
	register("plain", newPlainObfs)
}

type plain struct {
	ssr.ServerInfoForObfs
}

func newPlainObfs() IObfs {
	p := &plain{}
	return p
}

func (p *plain) SetServerInfo(s *ssr.ServerInfoForObfs) {
	p.ServerInfoForObfs = *s
}

func (p *plain) GetServerInfo() (s *ssr.ServerInfoForObfs) {
	return &p.ServerInfoForObfs
}

func (p *plain) Encode(data []byte) (encodedData []byte, err error) {
	return data, nil
}

func (p *plain) Decode(data []byte) ([]byte, uint64, error) {
	return data, 0, nil
}

func (p *plain) SetData(data interface{}) {

}

func (p *plain) GetData() interface{} {
	return nil
}
