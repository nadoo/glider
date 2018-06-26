// +build linux

// ref: https://www.kernel.org/doc/Documentation/networking/tproxy.txt
// @LiamHaworth: https://github.com/LiamHaworth/go-tproxy/blob/master/tproxy_udp.go

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"syscall"
	"unsafe"
)

// TProxy struct
type TProxy struct {
	dialer Dialer
	addr   string
}

// NewTProxy returns a tproxy.
func NewTProxy(addr string, dialer Dialer) (*TProxy, error) {
	s := &TProxy{
		dialer: dialer,
		addr:   addr,
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

	lc, err := net.ListenUDP("udp", laddr)
	if err != nil {
		logf("proxy-tproxy failed to listen on %s: %v", s.addr, err)
		return
	}

	fd, err := lc.File()
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

	for {
		buf := make([]byte, 1024)
		_, srcAddr, dstAddr, err := ReadFromUDP(lc, buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				logf("proxy-tproxy temporary reading data error: %s", netErr)
				continue
			}

			logf("proxy-tproxy Unrecoverable error while reading data: %s", err)
			continue
		}

		logf("proxy-tproxy Accepting UDP connection from %s with destination of %s", srcAddr.String(), dstAddr.String())

	}

}

// ReadFromUDP reads a UDP packet from c, copying the payload into b.
// It returns the number of bytes copied into b and the return address
// that was on the packet.
//
// Out-of-band data is also read in so that the original destination
// address can be identified and parsed.
func ReadFromUDP(conn *net.UDPConn, b []byte) (int, *net.UDPAddr, *net.UDPAddr, error) {
	oob := make([]byte, 1024)
	n, oobn, _, addr, err := conn.ReadMsgUDP(b, oob)
	if err != nil {
		return 0, nil, nil, err
	}

	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return 0, nil, nil, fmt.Errorf("parsing socket control message: %s", err)
	}

	var originalDst *net.UDPAddr
	for _, msg := range msgs {
		if msg.Header.Level == syscall.SOL_IP && msg.Header.Type == syscall.IP_RECVORIGDSTADDR {
			originalDstRaw := &syscall.RawSockaddrInet4{}
			if err = binary.Read(bytes.NewReader(msg.Data), binary.LittleEndian, originalDstRaw); err != nil {
				return 0, nil, nil, fmt.Errorf("reading original destination address: %s", err)
			}

			switch originalDstRaw.Family {
			case syscall.AF_INET:
				pp := (*syscall.RawSockaddrInet4)(unsafe.Pointer(originalDstRaw))
				p := (*[2]byte)(unsafe.Pointer(&pp.Port))
				originalDst = &net.UDPAddr{
					IP:   net.IPv4(pp.Addr[0], pp.Addr[1], pp.Addr[2], pp.Addr[3]),
					Port: int(p[0])<<8 + int(p[1]),
				}

			case syscall.AF_INET6:
				pp := (*syscall.RawSockaddrInet6)(unsafe.Pointer(originalDstRaw))
				p := (*[2]byte)(unsafe.Pointer(&pp.Port))
				originalDst = &net.UDPAddr{
					IP:   net.IP(pp.Addr[:]),
					Port: int(p[0])<<8 + int(p[1]),
					Zone: strconv.Itoa(int(pp.Scope_id)),
				}

			default:
				return 0, nil, nil, fmt.Errorf("original destination is an unsupported network family")
			}
		}
	}

	if originalDst == nil {
		return 0, nil, nil, fmt.Errorf("unable to obtain original destination: %s", err)
	}

	return n, addr, originalDst, nil
}
