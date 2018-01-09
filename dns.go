// https://tools.ietf.org/html/rfc1035

package main

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
)

// DNSUDPHeaderLen is the length of UDP dns msg header
const DNSUDPHeaderLen = 12

// DNSTCPHeaderLen is the length of TCP dns msg header
const DNSTCPHeaderLen = 2 + DNSUDPHeaderLen

// DNSUDPMaxLen is the max size of udp dns request.
// https://tools.ietf.org/html/rfc1035#section-4.2.1
// Messages carried by UDP are restricted to 512 bytes (not counting the IP
// or UDP headers).  Longer messages are truncated and the TC bit is set in
// the header.
// TODO: If the request length > 512 then the client will send TCP packets instead,
// so we should also serve tcp requests.
const DNSUDPMaxLen = 512

// DNSQueryTypeA ipv4
const DNSQueryTypeA = 1

// DNSQueryTypeAAAA ipv6
const DNSQueryTypeAAAA = 28

type dnsQuery struct {
	DomainName string
	QueryType  uint16
	QueryClass uint16
	Offset     int
}

type dnsAnswer struct {
	// DomainName string
	QueryType  uint16
	QueryClass uint16
	TTL        uint32
	DataLength uint16
	Data       []byte

	IP string
}

// DNSAnswerHandler .
type DNSAnswerHandler func(domain, ip string) error

// DNS .
type DNS struct {
	*Forwarder        // as proxy client
	sDialer    Dialer // dialer for server

	tunnel bool

	dnsServer string

	dnsServerMap   map[string]string
	answerHandlers []DNSAnswerHandler
}

// NewDNS returns a dns forwarder. client[dns.udp] -> glider[tcp] -> forwarder[dns.tcp] -> remote dns addr
func NewDNS(addr, raddr string, sDialer Dialer, tunnel bool) (*DNS, error) {
	s := &DNS{
		Forwarder: NewForwarder(addr, nil),
		sDialer:   sDialer,

		tunnel: tunnel,

		dnsServer:    raddr,
		dnsServerMap: make(map[string]string),
	}

	return s, nil
}

// ListenAndServe .
func (s *DNS) ListenAndServe() {
	go s.ListenAndServeTCP()
	s.ListenAndServeUDP()
}

// ListenAndServeUDP .
func (s *DNS) ListenAndServeUDP() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		logf("proxy-dns failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	logf("proxy-dns listening on udp:%s", s.addr)

	for {
		data := make([]byte, DNSUDPMaxLen)

		n, clientAddr, err := c.ReadFrom(data)
		if err != nil {
			logf("proxy-dns DNS local read error: %v", err)
			continue
		}

		data = data[:n]

		go func() {

			_, respMsg := s.handleReqMsg(uint16(len(data)), data)

			_, err = c.WriteTo(respMsg, clientAddr)
			if err != nil {
				logf("proxy-dns error in local write: %s\n", err)
			}

			// logf("proxy-dns %s <-> %s, type: %d, %s: %s", clientAddr.String(), dnsServer, query.QueryType, domain, ip)

		}()
	}
}

// ListenAndServeTCP .
func (s *DNS) ListenAndServeTCP() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		logf("proxy-dns-tcp error: %v", err)
		return
	}

	logf("proxy-dns-tcp listening on tcp:%s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			logf("proxy-dns-tcp error: failed to accept: %v", err)
			continue
		}
		go s.ServeTCP(c)
	}
}

// ServeTCP .
func (s *DNS) ServeTCP(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	var reqLen uint16
	if err := binary.Read(c, binary.BigEndian, &reqLen); err != nil {
		logf("proxy-dns-tcp failed to read request length: %v", err)
		return
	}

	reqMsg := make([]byte, reqLen)
	_, err := io.ReadFull(c, reqMsg)
	if err != nil {
		logf("proxy-dns-tcp error in read reqMsg %s", err)
		return
	}

	respLen, respMsg := s.handleReqMsg(reqLen, reqMsg)

	if err := binary.Write(c, binary.BigEndian, respLen); err != nil {
		logf("proxy-dns-tcp error in local write respLen: %s\n", err)
	}
	if err := binary.Write(c, binary.BigEndian, respMsg); err != nil {
		logf("proxy-dns-tcp error in local write respMsg: %s\n", err)
	}

	// logf("proxy-dns-tcp %s <-> %s, type: %d, %s: %s", c.RemoteAddr(), dnsServer, query.QueryType, domain, ip)
}

// handle request msg and return response msg
func (s *DNS) handleReqMsg(reqLen uint16, reqMsg []byte) (respLen uint16, respMsg []byte) {

	query, err := parseQuery(reqMsg)
	if err != nil {
		logf("proxy-dns-tcp error in parseQuery reqMsg %s", err)
		return
	}

	dnsServer := s.GetServer(query.DomainName)
	if s.tunnel {
		dnsServer = s.dnsServer
	}

	rc, err := s.sDialer.NextDialer(query.DomainName+":53").Dial("tcp", dnsServer)
	if err != nil {
		logf("proxy-dns failed to connect to server %v: %v", dnsServer, err)
		return
	}
	defer rc.Close()

	if err := binary.Write(rc, binary.BigEndian, reqLen); err != nil {
		logf("proxy-dns failed to connect to server %v: %v", dnsServer, err)
	}
	if err := binary.Write(rc, binary.BigEndian, reqMsg); err != nil {
		logf("proxy-dns failed to connect to server %v: %v", dnsServer, err)
	}

	if err := binary.Read(rc, binary.BigEndian, &respLen); err != nil {
		logf("proxy-dns-tcp failed to read response length: %v", err)
		return
	}

	respMsg = make([]byte, respLen)
	_, err = io.ReadFull(rc, respMsg)
	if err != nil {
		logf("proxy-dns-tcp error in read respMsg %s\n", err)
		return
	}

	// fmt.Printf("dns resp len %d:\n%s\n\n", respLen, hex.Dump(respMsg[:]))

	var ip string
	if respLen > 0 {
		query, err := parseQuery(respMsg)
		if err != nil {
			logf("proxy-dns error in parseQuery respMsg %s", err)
			return
		}

		if (query.QueryType == DNSQueryTypeA || query.QueryType == DNSQueryTypeAAAA) &&
			len(respMsg) > query.Offset {

			answers := parseAnswers(respMsg[query.Offset:])

			for _, answer := range answers {
				if answer.IP != "" {
					ip += answer.IP + ","
				}

				for _, h := range s.answerHandlers {
					h(query.DomainName, answer.IP)
				}
			}
		}

	}

	return
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

// AddAnswerHandler .
func (s *DNS) AddAnswerHandler(h DNSAnswerHandler) {
	s.answerHandlers = append(s.answerHandlers, h)
}

func parseQuery(p []byte) (*dnsQuery, error) {
	q := &dnsQuery{}

	var i int
	var domain []byte
	for i = DNSUDPHeaderLen; i < len(p); {
		l := int(p[i])

		if l == 0 {
			i++
			break
		}

		domain = append(domain, p[i+1:i+l+1]...)
		domain = append(domain, '.')

		i = i + l + 1
	}

	q.DomainName = string(domain[:len(domain)-1])

	if len(p) < i+4 {
		return nil, errors.New("parseQuery error, not enough data")
	}

	q.QueryType = binary.BigEndian.Uint16(p[i:])
	q.QueryClass = binary.BigEndian.Uint16(p[i+2:])
	q.Offset = i + 4

	return q, nil
}

func parseAnswers(p []byte) []*dnsAnswer {
	var answers []*dnsAnswer

	for i := 0; i < len(p); {

		// https://tools.ietf.org/html/rfc1035#section-4.1.4
		// "Message compression",
		// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
		// | 1  1|                OFFSET                   |
		// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+

		if p[i]>>6 == 3 {
			i += 2
		} else {
			// TODO: none compressed query name and Additional records will be ignored
			break
		}

		answer := &dnsAnswer{}

		answer.QueryType = binary.BigEndian.Uint16(p[i:])
		answer.QueryClass = binary.BigEndian.Uint16(p[i+2:])
		answer.TTL = binary.BigEndian.Uint32(p[i+4:])
		answer.DataLength = binary.BigEndian.Uint16(p[i+8:])
		answer.Data = p[i+10 : i+10+int(answer.DataLength)]

		if answer.QueryType == DNSQueryTypeA {
			answer.IP = net.IP(answer.Data[:net.IPv4len]).String()
		} else if answer.QueryType == DNSQueryTypeAAAA {
			answer.IP = net.IP(answer.Data[:net.IPv6len]).String()
		}

		answers = append(answers, answer)

		i = i + 10 + int(answer.DataLength)
	}

	return answers
}
