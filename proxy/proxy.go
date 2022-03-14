package proxy

import (
	"net"
	"strings"
)

// Proxy is a dialer manager.
type Proxy interface {
	// Dial connects to the given address via the proxy.
	Dial(network, addr string) (c net.Conn, dialer Dialer, err error)

	// DialUDP connects to the given address via the proxy.
	DialUDP(network, addr string) (pc net.PacketConn, dialer UDPDialer, err error)

	// Get the dialer by dstAddr.
	NextDialer(dstAddr string) Dialer

	// Record records result while using the dialer from proxy.
	Record(dialer Dialer, success bool)
}

var (
	msg    strings.Builder
	usages = make(map[string]string)
)

// AddUsage adds help message for the named proxy.
func AddUsage(name, usage string) {
	usages[name] = usage
	msg.WriteString(usage)
	msg.WriteString("\n--")
}

// Usage returns help message of the named proxy.
func Usage(name string) string {
	if name == "all" {
		return msg.String()
	}

	if usage, ok := usages[name]; ok {
		return usage
	}

	return "can not find usage for: " + name
}
