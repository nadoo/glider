package protocol

import (
	"github.com/sun8911879/shadowsocksR/ssr"
)

func init() {
	register("origin", NewOrigin)
}

type origin struct {
	ssr.ServerInfoForObfs
}

func NewOrigin() IProtocol {
	a := &origin{}
	return a
}

func (o *origin) SetServerInfo(s *ssr.ServerInfoForObfs) {
	o.ServerInfoForObfs = *s
}

func (o *origin) GetServerInfo() (s *ssr.ServerInfoForObfs) {
	return &o.ServerInfoForObfs
}

func (o *origin) PreEncrypt(data []byte) (encryptedData []byte, err error) {
	return data, nil
}

func (o *origin) PostDecrypt(data []byte) ([]byte, int, error) {
	return data, 0, nil
}

func (o *origin) SetData(data interface{}) {

}

func (o *origin) GetData() interface{} {
	return nil
}
