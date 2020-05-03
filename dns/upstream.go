package dns

import "sync/atomic"

// UpStream is a dns upstream.
type UpStream struct {
	index   uint32
	servers []string
}

// NewUpStream returns a new UpStream.
func NewUpStream(servers []string) *UpStream {
	return &UpStream{servers: servers}
}

// Server returns a dns server.
func (u *UpStream) Server() string {
	return u.servers[atomic.LoadUint32(&u.index)%uint32(len(u.servers))]
}

// Switch switches to the next dns server.
func (u *UpStream) Switch() string {
	return u.servers[atomic.AddUint32(&u.index, 1)%uint32(len(u.servers))]
}

// SwitchIf switches to the next dns server if needed.
func (u *UpStream) SwitchIf(server string) string {
	if u.Server() == server {
		return u.Switch()
	}
	return u.Server()
}

// Len returns the number of dns servers.
func (u *UpStream) Len() int {
	return len(u.servers)
}
