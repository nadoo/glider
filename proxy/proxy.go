package proxy

import "net"

// Proxy is a dialer manager.
type Proxy interface {
	// Dial connects to the given address via the proxy.
	Dial(network, addr string) (c net.Conn, dialer Dialer, err error)

	// DialUDP connects to the given address via the proxy.
	DialUDP(network, addr string) (pc net.PacketConn, dialer UDPDialer, writeTo net.Addr, err error)

	// Get the dialer by dstAddr.
	NextDialer(dstAddr string) Dialer

	// Record records result while using the dialer from proxy.
	Record(dialer Dialer, success bool)
}
