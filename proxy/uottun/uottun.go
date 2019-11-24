package uottun

import (
	"errors"
	"io/ioutil"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/dongxinb/glider/common/conn"
	"github.com/dongxinb/glider/common/log"
	"github.com/dongxinb/glider/proxy"
)

// UoTTun is a base udp over tcp tunnel struct.
type UoTTun struct {
	proxy proxy.Proxy
	addr  string

	raddr string
}

func init() {
	proxy.RegisterServer("uottun", NewUoTTunServer)
}

// NewUoTTun returns a UoTTun proxy.
func NewUoTTun(s string, p proxy.Proxy) (*UoTTun, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	d := strings.Split(addr, "=")
	if len(d) < 2 {
		return nil, errors.New("error in strings.Split")
	}

	ut := &UoTTun{
		proxy: p,
		addr:  d[0],
		raddr: d[1],
	}

	return ut, nil
}

// NewUoTTunServer returns a uot tunnel server.
func NewUoTTunServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewUoTTun(s, p)
}

// ListenAndServe listen and serve on tcp.
func (s *UoTTun) ListenAndServe() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("[uottun] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	log.F("[uottun] listening UDP on %s", s.addr)

	buf := make([]byte, conn.UDPBufSize)

	for {
		n, clientAddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[uottun] read error: %v", err)
			continue
		}

		rc, p, err := s.proxy.Dial("uot", s.raddr)
		if err != nil {
			log.F("[uottun] failed to connect to server %v: %v", s.raddr, err)
			continue
		}

		go func() {
			// no remote forwarder, just a local udp forwarder
			if urc, ok := rc.(*net.UDPConn); ok {
				conn.RelayUDP(c, clientAddr, urc, 2*time.Minute)
				urc.Close()
				return
			}

			// remote forwarder, udp over tcp
			resp, err := ioutil.ReadAll(rc)
			if err != nil {
				log.F("error in ioutil.ReadAll: %s\n", err)
				return
			}
			rc.Close()
			c.WriteTo(resp, clientAddr)
		}()

		_, err = rc.Write(buf[:n])
		if err != nil {
			log.F("[uottun] remote write error: %v", err)
			continue
		}

		log.F("[uottun] %s <-> %s via %s", clientAddr, s.raddr, p)
	}
}

// Serve is not allowed to be called directly.
func (s *UoTTun) Serve(c net.Conn) {
	// TODO
	log.F("[uottun] func Serve: can not be called directly")
}
