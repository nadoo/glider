package obfs

import (
	"github.com/nadoo/glider/proxy/ssr/internal/ssr"
)

func init() {
	register("plain", newPlainObfs)
}

type plain struct {
	ssr.ServerInfo
}

func newPlainObfs() IObfs {
	p := &plain{}
	return p
}

func (p *plain) SetServerInfo(s *ssr.ServerInfo) {
	p.ServerInfo = *s
}

func (p *plain) GetServerInfo() (s *ssr.ServerInfo) {
	return &p.ServerInfo
}

func (p *plain) Encode(data []byte) (encodedData []byte, err error) {
	return data, nil
}

func (p *plain) Decode(data []byte) (decodedData []byte, needSendBack bool, err error) {
	return data, false, nil
}

func (p *plain) SetData(data any) {

}

func (p *plain) GetData() any {
	return nil
}

func (p *plain) GetOverhead() int {
	return 0
}
