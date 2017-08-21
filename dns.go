// https://tools.ietf.org/html/rfc1035

package main

import (
	"encoding/binary"
	"io/ioutil"
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
	DomainName string
	QueryType  uint16
	QueryClass uint16
	TTL        uint32
	DataLength uint16
	Data       []byte

	IP string
}

// DNS .
type DNS struct {
	*proxy
	dnsServer string

	dnsServerMap map[string]string
}

// NewDNS returns a dns forwarder. client -> dns.udp -> glider -> forwarder -> remote dns addr
func NewDNS(addr, raddr string, upProxy Proxy) (*DNS, error) {
	s := &DNS{
		proxy:        NewProxy(addr, upProxy),
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
		data := make([]byte, DNSUDPMaxLen)

		n, clientAddr, err := l.ReadFrom(data)
		if err != nil {
			logf("DNS local read error: %v", err)
			continue
		}

		data = data[:n]

		go func() {
			// TODO: check domain rules and get a proper upstream name server.
			query := parseQuery(data)
			domain := query.DomainName

			dnsServer := s.GetServer(domain)
			// TODO: check here
			rc, err := s.GetProxy(domain+":53").Dial("tcp", dnsServer)
			if err != nil {
				logf("failed to connect to server %v: %v", dnsServer, err)
				return
			}
			defer rc.Close()

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

			var ip string
			// length is not needed in udp dns response. (2 bytes)
			// SEE RFC1035, section 4.2.2 TCP: The message is prefixed with a two byte length field which gives the message length, excluding the two byte length field.
			if len(resp) > 2 {
				msg := resp[2:]
				// TODO: Get IP from response, check and add to ipset
				query := parseQuery(msg)
				if len(msg) > query.Offset {
					answers := parseAnswers(msg[query.Offset:])
					for _, answer := range answers {
						if answer.IP != "" {
							ip += answer.IP + ","
						}

					}

				}

				_, err = l.WriteTo(msg, clientAddr)
				if err != nil {
					logf("error in local write: %s\n", err)
				}
			}

			logf("proxy-dns %s, %s <-> %s, ip: %s", domain, clientAddr.String(), dnsServer, ip)

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
		l := int(p[i])

		if l == 0 {
			i++
			break
		}

		answer := &dnsAnswer{}
		answer.QueryType = binary.BigEndian.Uint16(p[i+2:])
		answer.QueryClass = binary.BigEndian.Uint16(p[i+4:])
		answer.TTL = binary.BigEndian.Uint32(p[i+6:])
		answer.DataLength = binary.BigEndian.Uint16(p[i+10:])
		answer.Data = p[i+12 : i+12+int(answer.DataLength)]

		if answer.QueryType == DNSQueryTypeA {
			answer.IP = net.IP(answer.Data[:net.IPv4len]).String()
		} else if answer.QueryType == DNSQueryTypeAAAA {
			answer.IP = net.IP(answer.Data[:net.IPv6len]).String()
		}

		answers = append(answers, answer)

		i = i + 12 + int(answer.DataLength)
	}

	return answers
}
