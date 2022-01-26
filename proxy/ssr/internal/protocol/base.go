package protocol

import (
	"strings"
	"sync"

	"github.com/nadoo/glider/proxy/ssr/internal/ssr"
	"github.com/nadoo/glider/proxy/ssr/internal/tools"
)

type creator func() IProtocol

var (
	creatorMap = make(map[string]creator)
)

type hmacMethod func(key []byte, data []byte) []byte
type hashDigestMethod func(data []byte) []byte
type rndMethod func(dataLength int, random *tools.Shift128plusContext, lastHash []byte, dataSizeList, dataSizeList2 []int, overhead int) int

type IProtocol interface {
	SetServerInfo(s *ssr.ServerInfo)
	GetServerInfo() *ssr.ServerInfo
	PreEncrypt(data []byte) ([]byte, error)
	PostDecrypt(data []byte) ([]byte, int, error)
	SetData(data any)
	GetData() any
	GetOverhead() int
}

type AuthData struct {
	clientID     []byte
	connectionID uint32
	mutex        sync.Mutex
}

func register(name string, c creator) {
	creatorMap[name] = c
}

func NewProtocol(name string) IProtocol {
	c, ok := creatorMap[strings.ToLower(name)]
	if ok {
		return c()
	}
	return nil
}
