package tproxy

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// ref: https://github.com/LiamHaworth/go-tproxy/blob/master/tproxy_udp.go
// MIT License by @LiamHaworth

// ListenUDP acts like net.ListenUDP but returns an conn with IP_TRANSPARENT option.
func ListenUDP(network string, laddr *net.UDPAddr) (*net.UDPConn, error) {
	listener, err := net.ListenUDP(network, laddr)
	if err != nil {
		return nil, err
	}

	fileDescriptorSource, err := listener.File()
	if err != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: laddr, Err: fmt.Errorf("get file descriptor: %s", err)}
	}
	defer fileDescriptorSource.Close()

	fileDescriptor := int(fileDescriptorSource.Fd())

	if err = syscall.SetsockoptInt(fileDescriptor, syscall.SOL_IP, syscall.IP_TRANSPARENT, 1); err != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: laddr, Err: fmt.Errorf("set socket option: IP_TRANSPARENT: %s", err)}
	}

	if laddr.IP == nil || laddr.IP.To4() != nil {
		if err = syscall.SetsockoptInt(fileDescriptor, syscall.SOL_IP, syscall.IP_RECVORIGDSTADDR, 1); err != nil {
			return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: laddr, Err: fmt.Errorf("set socket option: IP_RECVORIGDSTADDR: %s", err)}
		}
	}

	if laddr.IP == nil || laddr.IP.To4() == nil {
		if err = syscall.SetsockoptInt(fileDescriptor, syscall.SOL_IPV6, unix.IPV6_RECVORIGDSTADDR, 1); err != nil {
			return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: laddr, Err: fmt.Errorf("set socket option: IPV6_RECVORIGDSTADDR: %s", err)}
		}
	}

	return listener, nil
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

	for _, msg := range msgs {
		if msg.Header.Level == syscall.SOL_IP && msg.Header.Type == syscall.IP_RECVORIGDSTADDR {
			ip := net.IP(msg.Data[4:8])
			port := binary.BigEndian.Uint16(msg.Data[2:4])
			return n, addr, &net.UDPAddr{IP: ip, Port: int(port)}, nil
		}
		if msg.Header.Level == syscall.SOL_IPV6 && msg.Header.Type == unix.IPV6_RECVORIGDSTADDR {
			ip := net.IP(msg.Data[8:24])
			port := binary.BigEndian.Uint16(msg.Data[2:4])
			return n, addr, &net.UDPAddr{IP: ip, Port: int(port)}, nil
		}
	}

	return 0, nil, nil, fmt.Errorf("unable to obtain original destination: %s", err)
}

// ListenPacket acts like net.ListenPacket but the addr could be non-local.
func ListenPacket(addr *net.UDPAddr) (net.PacketConn, error) {
	var af int
	var sockaddr syscall.Sockaddr

	if len(addr.IP) == 4 {
		af = syscall.AF_INET
		sockaddr = &syscall.SockaddrInet4{Port: addr.Port}
		copy(sockaddr.(*syscall.SockaddrInet4).Addr[:], addr.IP)
	} else {
		af = syscall.AF_INET6
		sockaddr = &syscall.SockaddrInet6{Port: addr.Port}
		copy(sockaddr.(*syscall.SockaddrInet6).Addr[:], addr.IP)
	}

	var fd int
	var err error

	if fd, err = syscall.Socket(af, syscall.SOCK_DGRAM, 0); err != nil {
		return nil, &net.OpError{Op: "fake", Err: fmt.Errorf("socket open: %s", err)}
	}

	if err = syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_TRANSPARENT, 1); err != nil {
		syscall.Close(fd)
		return nil, &net.OpError{Op: "fake", Err: fmt.Errorf("set socket option: IP_TRANSPARENT: %s", err)}
	}

	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)

	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)

	if err = syscall.Bind(fd, sockaddr); err != nil {
		syscall.Close(fd)
		return nil, &net.OpError{Op: "fake", Err: fmt.Errorf("socket bind: %s", err)}
	}

	fdFile := os.NewFile(uintptr(fd), fmt.Sprintf("net-udp-listen-%s", addr.String()))
	defer fdFile.Close()

	packetConn, err := net.FilePacketConn(fdFile)
	if err != nil {
		syscall.Close(fd)
		return nil, &net.OpError{Op: "fake", Err: fmt.Errorf("convert file descriptor to connection: %s", err)}
	}

	return packetConn, nil
}
