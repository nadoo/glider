// https://tools.ietf.org/html/rfc1035

package main

import (
	"encoding/binary"
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

	dnsServer string

	dnsServerMap   map[string]string
	answerHandlers []DNSAnswerHandler
}

// NewDNS returns a dns forwarder. client[dns.udp] -> glider[tcp] -> forwarder[dns.tcp] -> remote dns addr
func NewDNS(addr, raddr string, sDialer Dialer) (*DNS, error) {
	s := &DNS{
		Forwarder: NewForwarder(addr, nil),
		sDialer:   sDialer,

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

	logf("proxy-dns listening UDP on %s", s.addr)

	for {
		data := make([]byte, DNSUDPMaxLen)

		n, clientAddr, err := c.ReadFrom(data)
		if err != nil {
			logf("proxy-dns DNS local read error: %v", err)
			continue
		}

		data = data[:n]

		go func() {
			query := parseQuery(data)
			domain := query.DomainName

			dnsServer := s.dnsServer
			if dnsServer == "" {
				dnsServer = s.GetServer(domain)
			}

			rc, err := s.sDialer.NextDialer(domain+":53").Dial("tcp", dnsServer)
			if err != nil {
				logf("proxy-dns failed to connect to server %v: %v", dnsServer, err)
				return
			}
			defer rc.Close()

			// 2 bytes length after tcp header, before dns message
			reqLen := make([]byte, 2)
			binary.BigEndian.PutUint16(reqLen, uint16(len(data)))
			rc.Write(reqLen)
			rc.Write(data)

			// fmt.Printf("dns req len %d:\n%s\n\n", reqLen, hex.Dump(data[:]))

			var respLen uint16
			err = binary.Read(rc, binary.BigEndian, &respLen)
			if err != nil {
				logf("proxy-dns error in read respLen %s\n", err)
				return
			}

			respMsg := make([]byte, respLen)
			_, err = io.ReadFull(rc, respMsg)
			if err != nil {
				logf("proxy-dns error in read respMsg %s\n", err)
				return
			}

			// fmt.Printf("dns resp len %d:\n%s\n\n", respLen, hex.Dump(respMsg[:]))

			var ip string
			// length is not needed in udp dns response. (2 bytes)
			// SEE RFC1035, section 4.2.2 TCP: The message is prefixed with a two byte length field which gives the message length, excluding the two byte length field.
			if respLen > 0 {

				// run handle functions before send to client so RULE and IPSET can take effect
				// TODO: add PRE_HANDLERS
				query := parseQuery(respMsg)
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

				_, err = c.WriteTo(respMsg, clientAddr)
				if err != nil {
					logf("proxy-dns error in local write: %s\n", err)
				}
			}

			logf("proxy-dns %s <-> %s, type: %d, %s: %s", clientAddr.String(), dnsServer, query.QueryType, domain, ip)

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

	logf("proxy-dns-tcp listening TCP on %s", s.addr)

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
		logf("proxy-dns-tcp error in read reqMsg %s\n", err)
		return
	}

	query := parseQuery(reqMsg)
	domain := query.DomainName

	dnsServer := s.dnsServer
	if dnsServer == "" {
		dnsServer = s.GetServer(domain)
	}

	rc, err := s.sDialer.NextDialer(domain+":53").Dial("tcp", dnsServer)
	if err != nil {
		logf("proxy-dns failed to connect to server %v: %v", dnsServer, err)
		return
	}
	defer rc.Close()

	binary.Write(rc, binary.BigEndian, reqLen)
	binary.Write(rc, binary.BigEndian, reqMsg)

	var respLen uint16
	if err := binary.Read(rc, binary.BigEndian, &respLen); err != nil {
		logf("proxy-dns-tcp failed to read response length: %v", err)
		return
	}

	respMsg := make([]byte, respLen)
	_, err = io.ReadFull(rc, respMsg)
	if err != nil {
		logf("proxy-dns-tcp error in read respMsg %s\n", err)
		return
	}

	var ip string
	if respLen > 0 {
		query := parseQuery(respMsg)
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

		binary.Write(c, binary.BigEndian, respLen)
		binary.Write(c, binary.BigEndian, respMsg)
	}

	logf("proxy-dns-tcp %s <-> %s, type: %d, %s: %s", c.RemoteAddr(), dnsServer, query.QueryType, domain, ip)

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

func parseQuery(p []byte) *dnsQuery {
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
	q.QueryType = binary.BigEndian.Uint16(p[i:])
	q.QueryClass = binary.BigEndian.Uint16(p[i+2:])
	q.Offset = i + 4

	return q
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
