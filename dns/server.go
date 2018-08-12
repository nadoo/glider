package dns

import (
	"encoding/binary"
	"io"
	"net"
	"time"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// conn timeout, seconds
const timeout = 30

// Server is a dns server struct
type Server struct {
	addr string
	// Client is used to communicate with upstream dns servers
	*Client
}

// NewServer returns a new dns server
func NewServer(addr string, dialer proxy.Dialer, config *Config) (*Server, error) {
	c, err := NewClient(dialer, config)
	s := &Server{
		addr:   addr,
		Client: c,
	}

	return s, err
}

// ListenAndServe .
func (s *Server) ListenAndServe() {
	go s.ListenAndServeTCP()
	s.ListenAndServeUDP()
}

// ListenAndServeUDP .
func (s *Server) ListenAndServeUDP() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("[dns] failed to listen on %s, error: %v", s.addr, err)
		return
	}
	defer c.Close()

	log.F("[dns] listening UDP on %s", s.addr)

	for {
		reqBytes := make([]byte, 2+UDPMaxLen)
		n, caddr, err := c.ReadFrom(reqBytes[2:])
		if err != nil {
			log.F("[dns] local read error: %v", err)
			continue
		}

		reqLen := uint16(n)
		if reqLen <= HeaderLen+2 {
			log.F("[dns] not enough message data")
			continue
		}
		binary.BigEndian.PutUint16(reqBytes[:2], reqLen)

		go func() {
			respBytes, err := s.Client.Exchange(reqBytes[:2+n], caddr.String(), false)
			if err != nil {
				log.F("[dns] error in exchange: %s", err)
				return
			}

			_, err = c.WriteTo(respBytes[2:], caddr)
			if err != nil {
				log.F("[dns] error in local write: %s", err)
				return
			}

		}()
	}

}

// ListenAndServeTCP .
func (s *Server) ListenAndServeTCP() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("[dns]-tcp error: %v", err)
		return
	}

	log.F("[dns]-tcp listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[dns]-tcp error: failed to accept: %v", err)
			continue
		}
		go s.ServeTCP(c)
	}
}

// ServeTCP .
func (s *Server) ServeTCP(c net.Conn) {
	defer c.Close()

	c.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Second))

	var reqLen uint16
	if err := binary.Read(c, binary.BigEndian, &reqLen); err != nil {
		log.F("[dns]-tcp failed to get request length: %v", err)
		return
	}

	reqBytes := make([]byte, reqLen+2)
	_, err := io.ReadFull(c, reqBytes[2:])
	if err != nil {
		log.F("[dns]-tcp error in read reqBytes %s", err)
		return
	}

	binary.BigEndian.PutUint16(reqBytes[:2], reqLen)

	respBytes, err := s.Exchange(reqBytes, c.RemoteAddr().String(), true)
	if err != nil {
		log.F("[dns]-tcp error in exchange: %s", err)
		return
	}

	if err := binary.Write(c, binary.BigEndian, respBytes); err != nil {
		log.F("[dns]-tcp error in local write respBytes: %s", err)
		return
	}
}
