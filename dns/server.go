package dns

import (
	"encoding/binary"
	"net"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// Server is a dns server struct
type Server struct {
	addr string
	// Client is used to communicate with upstream dns servers
	*Client
}

// NewServer returns a new dns server
func NewServer(addr string, dialer proxy.Dialer, upServers ...string) (*Server, error) {
	c, err := NewClient(dialer, upServers...)
	s := &Server{
		addr:   addr,
		Client: c,
	}

	return s, err
}

// ListenAndServe .
func (s *Server) ListenAndServe() {
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
			respBytes, err := s.Client.Exchange(reqBytes[:2+n], caddr.String())
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
