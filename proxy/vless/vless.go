package vless

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"

	"github.com/nadoo/glider/proxy"
)

// Version of vless protocol.
const Version byte = 0

// CmdType is vless cmd type.
type CmdType byte

// CMD types.
const (
	CmdErr CmdType = 0
	CmdTCP CmdType = 1
	CmdUDP CmdType = 2
)

// VLess struct.
type VLess struct {
	dialer   proxy.Dialer
	proxy    proxy.Proxy
	addr     string
	uuid     [16]byte
	fallback string
}

func init() {
	proxy.RegisterDialer("vless", NewVLessDialer)
	proxy.RegisterServer("vless", NewVLessServer)
}

// NewVLess returns a vless proxy.
func NewVLess(s string, d proxy.Dialer, p proxy.Proxy) (*VLess, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	addr := u.Host
	uuid, err := StrToUUID(u.User.Username())
	if err != nil {
		return nil, err
	}

	query := u.Query()
	v := &VLess{
		dialer:   d,
		proxy:    p,
		addr:     addr,
		uuid:     uuid,
		fallback: query.Get("fallback"),
	}

	return v, nil
}

// StrToUUID converts string to uuid.
func StrToUUID(s string) (uuid [16]byte, err error) {
	if len(s) >= 1 && len(s) <= 30 {
		h := sha1.New()
		h.Write(uuid[:])
		h.Write([]byte(s))
		u := h.Sum(nil)[:16]
		u[6] = (u[6] & 0x0f) | (5 << 4)
		u[8] = (u[8]&(0xff>>2) | (0x02 << 6))
		copy(uuid[:], u)
		return
	}
	b := []byte(strings.Replace(s, "-", "", -1))
	if len(b) != 32 {
		return uuid, errors.New("invalid UUID: " + s)
	}
	_, err = hex.Decode(uuid[:], b)
	return
}

func init() {
	proxy.AddUsage("vless", `
VLESS scheme:
  vless://uuid@host:port[?fallback=127.0.0.1:80]
`)
}
