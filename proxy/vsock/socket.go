// Source code from:
// https://github.com/linuxkit/virtsock/tree/master/pkg/vsock

package vsock

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Addr represents the address of a vsock end point.
type Addr struct {
	CID  uint32
	Port uint32
}

// Network returns the network type for a Addr
func (a Addr) Network() string {
	return "vsock"
}

// String returns a string representation of a Addr
func (a Addr) String() string {
	return fmt.Sprintf("%d:%d", a.CID, a.Port)
}

// Conn is a vsock connection which supports half-close.
type Conn interface {
	net.Conn
	CloseRead() error
	CloseWrite() error
	File() (*os.File, error)
}

// SocketMode is a NOOP on Linux.
func SocketMode(m string) {}

// Convert a generic unix.Sockaddr to a Addr.
func sockaddrToVsock(sa unix.Sockaddr) *Addr {
	switch sa := sa.(type) {
	case *unix.SockaddrVM:
		return &Addr{CID: sa.CID, Port: sa.Port}
	}
	return nil
}

// Closes fd, retrying EINTR
func closeFD(fd int) error {
	for {
		if err := unix.Close(fd); err != nil {
			if errno, ok := err.(syscall.Errno); ok && errno == syscall.EINTR {
				continue
			}
			return fmt.Errorf("failed to close() fd %d: %w", fd, err)
		}
		break
	}
	return nil
}

// Dial connects to the CID.Port via virtio sockets.
func Dial(cid, port uint32) (Conn, error) {
	fd, err := syscall.Socket(unix.AF_VSOCK, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("Failed to create AF_VSOCK socket: %w", err)
	}
	sa := &unix.SockaddrVM{CID: cid, Port: port}
	// Retry connect in a loop if EINTR is encountered.
	for {
		if err := unix.Connect(fd, sa); err != nil {
			if errno, ok := err.(syscall.Errno); ok && errno == syscall.EINTR {
				continue
			}
			// Trying not to leak fd here
			_ = closeFD(fd)
			return nil, fmt.Errorf("failed connect() to %d:%d: %w", cid, port, err)
		}
		break
	}
	return newVsockConn(uintptr(fd), nil, &Addr{cid, port}), nil
}

// Listen returns a net.Listener which can accept connections on the given cid and port.
func Listen(cid, port uint32) (net.Listener, error) {
	fd, err := syscall.Socket(unix.AF_VSOCK, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}

	sa := &unix.SockaddrVM{CID: cid, Port: port}
	if err = unix.Bind(fd, sa); err != nil {
		return nil, fmt.Errorf("bind() to %d:%d failed: %w", cid, port, err)
	}

	err = syscall.Listen(fd, syscall.SOMAXCONN)
	if err != nil {
		return nil, fmt.Errorf("listen() on %d:%d failed: %w", cid, port, err)
	}
	return &vsockListener{fd, Addr{cid, port}}, nil
}

// ContextID retrieves the local context ID for this system.
func ContextID() (uint32, error) {
	f, err := os.Open("/dev/vsock")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	return unix.IoctlGetUint32(int(f.Fd()), unix.IOCTL_VM_SOCKETS_GET_LOCAL_CID)
}

type vsockListener struct {
	fd    int
	local Addr
}

// Accept accepts an incoming call and returns the new connection.
func (v *vsockListener) Accept() (net.Conn, error) {
	fd, sa, err := unix.Accept(v.fd)
	if err != nil {
		return nil, err
	}
	return newVsockConn(uintptr(fd), &v.local, sockaddrToVsock(sa)), nil
}

// Close closes the listening connection
func (v *vsockListener) Close() error {
	// Note this won't cause the Accept to unblock.
	return unix.Close(v.fd)
}

// Addr returns the address the Listener is listening on
func (v *vsockListener) Addr() net.Addr {
	return v.local
}

// a wrapper around FileConn which supports CloseRead and CloseWrite
type vsockConn struct {
	vsock  *os.File
	fd     uintptr
	local  *Addr
	remote *Addr
}

func newVsockConn(fd uintptr, local, remote *Addr) *vsockConn {
	vsock := os.NewFile(fd, fmt.Sprintf("vsock:%d", fd))
	return &vsockConn{vsock: vsock, fd: fd, local: local, remote: remote}
}

// LocalAddr returns the local address of a connection
func (v *vsockConn) LocalAddr() net.Addr {
	return v.local
}

// RemoteAddr returns the remote address of a connection
func (v *vsockConn) RemoteAddr() net.Addr {
	return v.remote
}

// Close closes the connection
func (v *vsockConn) Close() error {
	return v.vsock.Close()
}

// CloseRead shuts down the reading side of a vsock connection
func (v *vsockConn) CloseRead() error {
	return syscall.Shutdown(int(v.fd), syscall.SHUT_RD)
}

// CloseWrite shuts down the writing side of a vsock connection
func (v *vsockConn) CloseWrite() error {
	return syscall.Shutdown(int(v.fd), syscall.SHUT_WR)
}

// Read reads data from the connection
func (v *vsockConn) Read(buf []byte) (int, error) {
	return v.vsock.Read(buf)
}

// Write writes data over the connection
func (v *vsockConn) Write(buf []byte) (int, error) {
	return v.vsock.Write(buf)
}

// SetDeadline sets the read and write deadlines associated with the connection
func (v *vsockConn) SetDeadline(t time.Time) error {
	return nil // FIXME
}

// SetReadDeadline sets the deadline for future Read calls.
func (v *vsockConn) SetReadDeadline(t time.Time) error {
	return nil // FIXME
}

// SetWriteDeadline sets the deadline for future Write calls
func (v *vsockConn) SetWriteDeadline(t time.Time) error {
	return nil // FIXME
}

// File duplicates the underlying socket descriptor and returns it.
func (v *vsockConn) File() (*os.File, error) {
	// This is equivalent to dup(2) but creates the new fd with CLOEXEC already set.
	r0, _, e1 := syscall.Syscall(syscall.SYS_FCNTL, uintptr(v.vsock.Fd()), syscall.F_DUPFD_CLOEXEC, 0)
	if e1 != 0 {
		return nil, os.NewSyscallError("fcntl", e1)
	}
	return os.NewFile(r0, v.vsock.Name()), nil
}
