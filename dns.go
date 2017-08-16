// https://tools.ietf.org/html/rfc1035

package main

import (
	"encoding/binary"
	"io/ioutil"
	"net"
	"strings"
)

// UDPDNSHeaderLen is the length of UDP dns msg header
const UDPDNSHeaderLen = 12

// TCPDNSHEADERLen is the length of TCP dns msg header
const TCPDNSHEADERLen = 2 + UDPDNSHeaderLen

// MaxUDPDNSLen is the max size of udp dns request.
// https://tools.ietf.org/html/rfc1035#section-4.2.1
// Messages carried by UDP are restricted to 512 bytes (not counting the IP
// or UDP headers).  Longer messages are truncated and the TC bit is set in
// the header.
// TODO: If the request length > 512 then the client will send TCP packets instead,
// so we should also serve tcp requests.
const MaxUDPDNSLen = 512

type DNS struct {
	*proxy
	dnsServer string

	dnsServerMap map[string]string
}

// DNSForwarder returns a dns forwarder. client -> dns.udp -> glider -> forwarder -> remote dns addr
func DNSForwarder(addr, raddr string, upProxy Proxy) (*DNS, error) {
	s := &DNS{
		proxy:        newProxy(addr, upProxy),
		dnsServer:    raddr,
		dnsServerMap: make(map[string]string),
	}

	return s, nil
}

// ListenAndServe .
func (s *DNS) ListenAndServe() {
	l, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		logf("failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	logf("listening UDP on %s", s.addr)

	for {
		data := make([]byte, MaxUDPDNSLen)

		n, clientAddr, err := l.ReadFrom(data)
		if err != nil {
			logf("DNS local read error: %v", err)
			continue
		}

		data = data[:n]

		go func() {
			// TODO: check domain rules and get a proper upstream name server.
			domain := string(getDomain(data))

			dnsServer := s.GetServer(domain)
			// TODO: check here
			rc, err := s.GetProxy(domain+":53").GetProxy(domain+":53").Dial("tcp", dnsServer)
			if err != nil {
				logf("failed to connect to server %v: %v", dnsServer, err)
				return
			}
			defer rc.Close()

			logf("proxy-dns %s, %s <-> %s", domain, clientAddr.String(), dnsServer)

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

// SetServer .
func (s *DNS) SetServer(domain, server string) {
	s.dnsServerMap[domain] = server
}

// GetServer .
func (s *DNS) GetServer(domain string) string {

	domainParts := strings.Split(domain, ".")
	length := len(domainParts)
	for i := length - 2; i >= 0; i-- {
		domain := strings.Join(domainParts[i:length], ".")

		if server, ok := s.dnsServerMap[domain]; ok {
			return server
		}
	}

	return s.dnsServer
}

// getDomain from dns request playload, return []byte like:
// []byte{'w', 'w', 'w', '.', 'm', 's', 'n', '.', 'c', 'o', 'm', '.'}
// []byte("www.msn.com.")
func getDomain(p []byte) []byte {
	var ret []byte

	for i := UDPDNSHeaderLen; i < len(p); {
		l := int(p[i])

		if l == 0 {
			break
		}

		ret = append(ret, p[i+1:i+l+1]...)
		ret = append(ret, '.')

		i = i + l + 1
	}

	// TODO: check here
	// domain name could not be null, so the length of ret always >= 1?
	return ret[:len(ret)-1]
}
