package uottun

import (
	"io/ioutil"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// UoTTun udp over tcp tunnel
type UoTTun struct {
	dialer proxy.Dialer
	addr   string

	raddr string
}

func init() {
	proxy.RegisterServer("uottun", NewUoTTunServer)
}

// NewUoTTun returns a UoTTun proxy.
func NewUoTTun(s string, dialer proxy.Dialer) (*UoTTun, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	d := strings.Split(addr, "=")

	p := &UoTTun{
		dialer: dialer,
		addr:   d[0],
		raddr:  d[1],
	}

	return p, nil
}

// NewUoTTunServer returns a uot tunnel server.
func NewUoTTunServer(s string, dialer proxy.Dialer) (proxy.Server, error) {
	return NewUoTTun(s, dialer)
}

// ListenAndServe .
func (s *UoTTun) ListenAndServe() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("proxy-uottun failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	log.F("proxy-uottun listening UDP on %s", s.addr)

	buf := make([]byte, conn.UDPBufSize)

	for {
		n, clientAddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("proxy-uottun read error: %v", err)
			continue
		}

		rc, err := s.dialer.Dial("uot", s.raddr)
		if err != nil {
			log.F("proxy-uottun failed to connect to server %v: %v", s.raddr, err)
			continue
		}

		go func() {
			// no remote forwarder, just a local udp forwarder
			if urc, ok := rc.(*net.UDPConn); ok {
				conn.TimedCopy(c, clientAddr, urc, 2*time.Minute)
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
			log.F("proxy-uottun remote write error: %v", err)
			continue
		}

		log.F("proxy-uottun %s <-> %s", clientAddr, s.raddr)
	}
}
