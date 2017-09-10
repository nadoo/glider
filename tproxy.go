// +build linux

package main

import (
	"net"
	"syscall"
)

// TProxy struct
type TProxy struct {
	*Forwarder        // as client
	sDialer    Dialer // dialer for server
}

// NewTProxy returns a tproxy.
func NewTProxy(addr string, sDialer Dialer) (*TProxy, error) {
	s := &TProxy{
		Forwarder: NewForwarder(addr, nil),
		sDialer:   sDialer,
	}

	return s, nil
}

// ListenAndServe .
func (s *TProxy) ListenAndServe() {
	// go s.ListenAndServeTCP()
	s.ListenAndServeUDP()
}

// ListenAndServeTCP .
func (s *TProxy) ListenAndServeTCP() {
	logf("proxy-tproxy tcp mode not supported now, please use 'redir' instead")
}

// ListenAndServeUDP .
func (s *TProxy) ListenAndServeUDP() {
	laddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		logf("proxy-tproxy failed to resolve addr %s: %v", s.addr, err)
		return
	}

	listener, err := net.ListenUDP("udp", laddr)
	if err != nil {
		logf("proxy-tproxy failed to listen on %s: %v", s.addr, err)
		return
	}

	fd, err := listener.File()
	if err != nil {
		logf("proxy-tproxy failed to get file descriptor: %v", err)
		return
	}
	defer fd.Close()

	fileDescriptor := int(fd.Fd())
	if err = syscall.SetsockoptInt(fileDescriptor, syscall.SOL_IP, syscall.IP_TRANSPARENT, 1); err != nil {
		syscall.Close(fileDescriptor)
		logf("proxy-tproxy failed to set socket option IP_TRANSPARENT: %v", err)
		return
	}

	if err = syscall.SetsockoptInt(fileDescriptor, syscall.SOL_IP, syscall.IP_RECVORIGDSTADDR, 1); err != nil {
		syscall.Close(fileDescriptor)
		logf("proxy-tproxy failed to set socket option IP_RECVORIGDSTADDR: %v", err)
		return
	}

}
