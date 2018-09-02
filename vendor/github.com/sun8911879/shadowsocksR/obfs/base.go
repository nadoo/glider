package obfs

import (
	"strings"

	"github.com/sun8911879/shadowsocksR/ssr"
)

type creator func() IObfs

var (
	creatorMap = make(map[string]creator)
)

type IObfs interface {
	SetServerInfo(s *ssr.ServerInfoForObfs)
	GetServerInfo() (s *ssr.ServerInfoForObfs)
	Encode(data []byte) ([]byte, error)
	Decode(data []byte) ([]byte, uint64, error)
	SetData(data interface{})
	GetData() interface{}
}

func register(name string, c creator) {
	creatorMap[name] = c
}

// NewObfs create an obfs object by name and return as an IObfs interface
func NewObfs(name string) IObfs {
	c, ok := creatorMap[strings.ToLower(name)]
	if ok {
		return c()
	}
	return nil
}
