package dns

import (
	"net"
	"sync/atomic"
)

// UPStream is a dns upstream.
type UPStream struct {
	index   uint32
	servers []string
}

// NewUPStream returns a new UpStream.
func NewUPStream(servers []string) *UPStream {
	// default port for dns upstream servers
	for i, server := range servers {
		if _, port, _ := net.SplitHostPort(server); port == "" {
			servers[i] = net.JoinHostPort(server, "53")
		}
	}
	return &UPStream{servers: servers}
}

// Server returns a dns server.
func (u *UPStream) Server() string {
	return u.servers[atomic.LoadUint32(&u.index)%uint32(len(u.servers))]
}

// Switch switches to the next dns server.
func (u *UPStream) Switch() string {
	return u.servers[atomic.AddUint32(&u.index, 1)%uint32(len(u.servers))]
}

// SwitchIf switches to the next dns server if needed.
func (u *UPStream) SwitchIf(server string) string {
	if u.Server() == server {
		return u.Switch()
	}
	return u.Server()
}

// Len returns the number of dns servers.
func (u *UPStream) Len() int {
	return len(u.servers)
}
