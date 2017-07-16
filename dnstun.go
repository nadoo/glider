package main

import (
	"encoding/binary"
	"io/ioutil"
	"net"
)

type dnstun struct {
	Proxy
	addr  string
	raddr string
}

// DNSTunProxy returns a dns forwarder. client -> dns.udp -> glider -> forwarder -> remote dns addr
func DNSTunProxy(addr, raddr string, upProxy Proxy) (Proxy, error) {
	s := &dnstun{
		Proxy: upProxy,
		addr:  addr,
		raddr: raddr,
	}

	return s, nil
}

// ListenAndServe .
func (s *dnstun) ListenAndServe() {
	l, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		logf("failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	logf("listening UDP on %s", s.addr)

	for {
		data := make([]byte, 512)
		n, clientAddr, err := l.ReadFrom(data)
		if err != nil {
			logf("DNS local read error: %v", err)
			continue
		}

		data = data[:n]
		go func() {
			rc, err := s.GetProxy().Dial("tcp", s.raddr)
			if err != nil {
				logf("failed to connect to server %v: %v", s.raddr, err)
				return
			}
			defer rc.Close()

			logf("proxy-dnstun %s[dns.udp] <-> %s[dns.tcp]", clientAddr.String(), s.raddr)

			// 2 bytes length after tcp header, before dns message
			length := make([]byte, 2)
			binary.BigEndian.PutUint16(length, uint16(len(data)))
			rc.Write(length)
			rc.Write(data)

			resp, err := ioutil.ReadAll(rc)
			if err != nil {
				logf("error in ioutil.ReadAll: %s\n", err)
				return
			}

			// length is not needed in udp dns response. (2 bytes)
			// SEE RFC1035, section 4.2.2 TCP: The message is prefixed with a two byte length field which gives the message length, excluding the two byte length field.
			if len(resp) > 2 {
				msg := resp[2:]
				_, err = l.WriteTo(msg, clientAddr)
				if err != nil {
					logf("error in local write: %s\n", err)
				}
			}

		}()
	}
}
