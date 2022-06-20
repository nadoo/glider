package grpc

import (
	"context"
	"crypto/x509"
	"fmt"
	"github.com/Qv2ray/gun/pkg/cert"
	"github.com/Qv2ray/gun/pkg/proto"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// https://github.com/v2fly/v2ray-core/blob/v5.0.6/transport/internet/grpc/dial.go
var (
	globalCCMap    map[string]*grpc.ClientConn
	globalCCAccess sync.Mutex
)

type ccCanceller func()

type ClientConn struct {
	tun       proto.GunService_TunClient
	closer    context.CancelFunc
	muReading sync.Mutex // muReading protects reading
	muWriting sync.Mutex // muWriting protects writing
	muRecv    sync.Mutex // muReading protects recv
	muSend    sync.Mutex // muWriting protects send
	buf       []byte
	offset    int

	deadlineMu    sync.Mutex
	readDeadline  *time.Timer
	writeDeadline *time.Timer
	readClosed    chan struct{}
	writeClosed   chan struct{}
	closed        chan struct{}
}

func NewClientConn(tun proto.GunService_TunClient, closer context.CancelFunc) *ClientConn {
	return &ClientConn{
		tun:         tun,
		closer:      closer,
		readClosed:  make(chan struct{}),
		writeClosed: make(chan struct{}),
		closed:      make(chan struct{}),
	}
}

type RecvResp struct {
	hunk *proto.Hunk
	err  error
}

func (c *ClientConn) Read(p []byte) (n int, err error) {
	select {
	case <-c.readClosed:
		return 0, os.ErrDeadlineExceeded
	case <-c.closed:
		return 0, io.EOF
	default:
	}

	c.muReading.Lock()
	defer c.muReading.Unlock()
	if c.buf != nil {
		n = copy(p, c.buf[c.offset:])
		c.offset += n
		if c.offset == len(c.buf) {
			pool.PutBuffer(c.buf)
			c.buf = nil
		}
		return n, nil
	}
	// set 1 to avoid channel leak
	readDone := make(chan RecvResp, 1)
	// pass channel to the function to avoid closure leak
	go func(readDone chan RecvResp) {
		// FIXME: not really abort the send so there is some problems when recover
		c.muRecv.Lock()
		defer c.muRecv.Unlock()
		recv, e := c.tun.Recv()
		readDone <- RecvResp{
			hunk: recv,
			err:  e,
		}
	}(readDone)
	select {
	case <-c.readClosed:
		return 0, os.ErrDeadlineExceeded
	case <-c.closed:
		return 0, io.EOF
	case recvResp := <-readDone:
		err = recvResp.err
		if err != nil {
			if code := status.Code(err); code == codes.Unavailable || status.Code(err) == codes.OutOfRange {
				err = io.EOF
			}
			return 0, err
		}
		n = copy(p, recvResp.hunk.Data)
		c.buf = pool.GetBuffer(len(recvResp.hunk.Data) - n)
		copy(c.buf, recvResp.hunk.Data[n:])
		c.offset = 0
		return n, nil
	}
}

func (c *ClientConn) Write(p []byte) (n int, err error) {
	select {
	case <-c.writeClosed:
		return 0, os.ErrDeadlineExceeded
	case <-c.closed:
		return 0, io.EOF
	default:
	}

	c.muWriting.Lock()
	defer c.muWriting.Unlock()
	// set 1 to avoid channel leak
	sendDone := make(chan error, 1)
	// pass channel to the function to avoid closure leak
	go func(sendDone chan error) {
		// FIXME: not really abort the send so there is some problems when recover
		c.muSend.Lock()
		defer c.muSend.Unlock()
		e := c.tun.Send(&proto.Hunk{Data: p})
		sendDone <- e
	}(sendDone)
	select {
	case <-c.writeClosed:
		return 0, os.ErrDeadlineExceeded
	case <-c.closed:
		return 0, io.EOF
	case err = <-sendDone:
		if code := status.Code(err); code == codes.Unavailable || status.Code(err) == codes.OutOfRange {
			err = io.EOF
		}
		return len(p), err
	}
}

func (c *ClientConn) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	c.closer()
	return nil
}
func (c *ClientConn) CloseWrite() error {
	return c.tun.CloseSend()
}
func (c *ClientConn) LocalAddr() net.Addr {
	// FIXME
	return nil
}
func (c *ClientConn) RemoteAddr() net.Addr {
	p, _ := peer.FromContext(c.tun.Context())
	return p.Addr
}

func (c *ClientConn) SetDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	if now := time.Now(); t.After(now) {
		// refresh the deadline if the deadline has been exceeded
		select {
		case <-c.readClosed:
			c.readClosed = make(chan struct{})
		default:
		}
		select {
		case <-c.writeClosed:
			c.writeClosed = make(chan struct{})
		default:
		}
		// reset the deadline timer to make the c.readClosed and c.writeClosed with the new pointer (if it is)
		if c.readDeadline != nil {
			c.readDeadline.Stop()
		}
		c.readDeadline = time.AfterFunc(t.Sub(now), func() {
			c.deadlineMu.Lock()
			defer c.deadlineMu.Unlock()
			select {
			case <-c.readClosed:
			default:
				close(c.readClosed)
			}
		})
		if c.writeDeadline != nil {
			c.writeDeadline.Stop()
		}
		c.writeDeadline = time.AfterFunc(t.Sub(now), func() {
			c.deadlineMu.Lock()
			defer c.deadlineMu.Unlock()
			select {
			case <-c.writeClosed:
			default:
				close(c.writeClosed)
			}
		})
	} else {
		select {
		case <-c.readClosed:
		default:
			close(c.readClosed)
		}
		select {
		case <-c.writeClosed:
		default:
			close(c.writeClosed)
		}
	}
	return nil
}

func (c *ClientConn) SetReadDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	if now := time.Now(); t.After(now) {
		// refresh the deadline if the deadline has been exceeded
		select {
		case <-c.readClosed:
			c.readClosed = make(chan struct{})
		default:
		}
		// reset the deadline timer to make the c.readClosed and c.writeClosed with the new pointer (if it is)
		if c.readDeadline != nil {
			c.readDeadline.Stop()
		}
		c.readDeadline = time.AfterFunc(t.Sub(now), func() {
			c.deadlineMu.Lock()
			defer c.deadlineMu.Unlock()
			select {
			case <-c.readClosed:
			default:
				close(c.readClosed)
			}
		})
	} else {
		select {
		case <-c.readClosed:
		default:
			close(c.readClosed)
		}
	}
	return nil
}

func (c *ClientConn) SetWriteDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	if now := time.Now(); t.After(now) {
		// refresh the deadline if the deadline has been exceeded
		select {
		case <-c.writeClosed:
			c.writeClosed = make(chan struct{})
		default:
		}
		if c.writeDeadline != nil {
			c.writeDeadline.Stop()
		}
		c.writeDeadline = time.AfterFunc(t.Sub(now), func() {
			c.deadlineMu.Lock()
			defer c.deadlineMu.Unlock()
			select {
			case <-c.writeClosed:
			default:
				close(c.writeClosed)
			}
		})
	} else {
		select {
		case <-c.writeClosed:
		default:
			close(c.writeClosed)
		}
	}
	return nil
}

// NewGRPCDialer returns a gRPC proxy dialer.
func NewGRPCDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	w, err := NewGRPC(s, d, nil)
	if err != nil {
		return nil, fmt.Errorf("[gRPC] create instance error: %s", err)
	}

	if w.certFile != "" {
		certData, err := os.ReadFile(w.certFile)
		if err != nil {
			return nil, fmt.Errorf("[gRPC] read cert file error: %s", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(certData) {
			return nil, fmt.Errorf("[gRPC] can not append cert file: %s", w.certFile)
		}
		w.certPool = certPool
	} else {
		w.certPool, err = cert.GetSystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to get system certificate pool")
		}
	}

	return w, err
}

// Addr returns forwarder's address.
func (g *GRPC) Addr() string {
	if g.addr == "" {
		return g.dialer.Addr()
	}
	return g.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (g *GRPC) Dial(network string, address string) (net.Conn, error) {
	cc, cancel, err := getGrpcClientConn(g.dialer, g.addr, g.serverName, g.certPool, g.skipVerify)
	if err != nil {
		cancel()
		return nil, err
	}
	client := proto.NewGunServiceClient(cc)

	clientX := client.(proto.GunServiceClientX)
	serviceName := g.serviceName
	if serviceName == "" {
		serviceName = "GunService"
	}
	// ctx is the lifetime of the tun
	ctxStream, streamCloser := context.WithCancel(context.Background())
	tun, err := clientX.TunCustomName(ctxStream, serviceName)
	if err != nil {
		streamCloser()
		return nil, err
	}
	return NewClientConn(tun, streamCloser), nil
}

// DialUDP connects to the given address via the proxy.
func (g *GRPC) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}

func getGrpcClientConn(dialer proxy.Dialer, address string, serverName string, certPool *x509.CertPool, skipVerify bool) (*grpc.ClientConn, ccCanceller, error) {
	globalCCAccess.Lock()
	if globalCCMap == nil {
		globalCCMap = make(map[string]*grpc.ClientConn)
	}
	globalCCAccess.Unlock()

	canceller := func() {
		globalCCAccess.Lock()
		defer globalCCAccess.Unlock()
		globalCCMap[address].Close()
		delete(globalCCMap, address)
	}

	// TODO Should support chain proxy to the same destination
	globalCCAccess.Lock()
	if client, found := globalCCMap[address]; found && client.GetState() != connectivity.Shutdown {
		globalCCAccess.Unlock()
		return client, canceller, nil
	}
	globalCCAccess.Unlock()
	dialOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(certPool, serverName)),
		grpc.WithContextDialer(func(ctxGrpc context.Context, s string) (net.Conn, error) {
			return dialer.Dial("tcp", s)
		}), grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  500 * time.Millisecond,
				Multiplier: 1.5,
				Jitter:     0.2,
				MaxDelay:   19 * time.Second,
			},
			MinConnectTimeout: 5 * time.Second,
		}), grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	}
	if skipVerify {
		dialOptions = append(dialOptions, grpc.WithInsecure())
	}
	cc, err := grpc.Dial(address, dialOptions...)
	globalCCAccess.Lock()
	globalCCMap[address] = cc
	globalCCAccess.Unlock()
	return cc, canceller, err
}
