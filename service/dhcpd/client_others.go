//go:build !linux
// +build !linux

package dhcpd

func existsServer(iface string) (exists bool) {
	return false
}
