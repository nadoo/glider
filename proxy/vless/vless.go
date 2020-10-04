package vless

import (
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

	v := &VLess{
		dialer: d,
		proxy:  p,
		addr:   addr,
		uuid:   uuid,
	}

	v.fallback = "127.0.0.1:80"
	if custom := u.Query().Get("fallback"); custom != "" {
		v.fallback = custom
	}

	return v, nil
}

// StrToUUID converts string to uuid.
// s fomat: "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
func StrToUUID(s string) (uuid [16]byte, err error) {
	b := []byte(strings.Replace(s, "-", "", -1))
	if len(b) != 32 {
		return uuid, errors.New("invalid UUID: " + s)
	}
	_, err = hex.Decode(uuid[:], b)
	return
}
