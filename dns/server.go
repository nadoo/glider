package dns

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/proxy"
)

// conn timeout, in seconds.
const timeout = 30

// Server is a dns server struct.
type Server struct {
	addr string
	// Client is used to communicate with upstream dns servers
	*Client
}

// NewServer returns a new dns server.
func NewServer(addr string, p proxy.Proxy, config *Config) (*Server, error) {
	c, err := NewClient(p, config)
	if err != nil {
		return nil, err
	}

	s := &Server{
		addr:   addr,
		Client: c,
	}
	return s, nil
}

// Start starts the dns forwarding server.
// We use WaitGroup here to ensure both udp and tcp serer are completly running,
// so we can start any other services later, since they may rely on dns service.
func (s *Server) Start() {
	var wg sync.WaitGroup
	wg.Add(2)
	go s.ListenAndServeTCP(&wg)
	go s.ListenAndServeUDP(&wg)
	wg.Wait()
}

// ListenAndServeUDP listen and serves on udp port.
func (s *Server) ListenAndServeUDP(wg *sync.WaitGroup) {
	pc, err := net.ListenPacket("udp", s.addr)
	wg.Done()
	if err != nil {
		log.F("[dns] failed to listen on %s, error: %v", s.addr, err)
		return
	}
	defer pc.Close()

	log.F("[dns] listening UDP on %s", s.addr)

	for {
		reqBytes := pool.GetBuffer(UDPMaxLen)
		n, caddr, err := pc.ReadFrom(reqBytes)
		if err != nil {
			log.F("[dns] local read error: %v", err)
			pool.PutBuffer(reqBytes)
			continue
		}
		go s.ServePacket(pc, caddr, reqBytes[:n])
	}
}

// ServePacket serves dns packet conn.
func (s *Server) ServePacket(pc net.PacketConn, caddr net.Addr, reqBytes []byte) {
	respBytes, err := s.Exchange(reqBytes, caddr.String(), false)
	defer func() {
		pool.PutBuffer(reqBytes)
		pool.PutBuffer(respBytes)
	}()

	if err != nil {
		log.F("[dns] error in exchange for %s: %s", caddr, err)
		return
	}

	_, err = pc.WriteTo(respBytes, caddr)
	if err != nil {
		log.F("[dns] error in local write to %s: %s", caddr, err)
		return
	}
}

// ListenAndServeTCP listen and serves on tcp port.
func (s *Server) ListenAndServeTCP(wg *sync.WaitGroup) {
	l, err := net.Listen("tcp", s.addr)
	wg.Done()
	if err != nil {
		log.F("[dns-tcp] error: %v", err)
		return
	}
	defer l.Close()

	log.F("[dns-tcp] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[dns-tcp] error: failed to accept: %v", err)
			continue
		}
		go s.ServeTCP(c)
	}
}

// ServeTCP serves a dns tcp connection.
func (s *Server) ServeTCP(c net.Conn) {
	defer c.Close()

	c.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Second))

	var reqLen uint16
	if err := binary.Read(c, binary.BigEndian, &reqLen); err != nil {
		log.F("[dns-tcp] failed to get request length: %v", err)
		return
	}

	reqBytes := pool.GetBuffer(int(reqLen))
	defer pool.PutBuffer(reqBytes)

	_, err := io.ReadFull(c, reqBytes)
	if err != nil {
		log.F("[dns-tcp] error in read reqBytes %s", err)
		return
	}

	respBytes, err := s.Exchange(reqBytes, c.RemoteAddr().String(), true)
	defer pool.PutBuffer(respBytes)
	if err != nil {
		log.F("[dns-tcp] error in exchange: %s", err)
		return
	}

	lenBuf := pool.GetBuffer(2)
	defer pool.PutBuffer(lenBuf)

	binary.BigEndian.PutUint16(lenBuf, uint16(len(respBytes)))
	if _, err := (&net.Buffers{lenBuf, respBytes}).WriteTo(c); err != nil {
		log.F("[dns-tcp] error in write respBytes: %s", err)
	}
}
