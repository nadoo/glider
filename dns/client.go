package dns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// HandleFunc function handles the dns TypeA or TypeAAAA answer.
type HandleFunc func(Domain, ip string) error

// Config for dns.
type Config struct {
	Servers   []string
	Timeout   int
	MaxTTL    int
	MinTTL    int
	Records   []string
	AlwaysTCP bool
}

// Client is a dns client struct.
type Client struct {
	dialer      proxy.Dialer
	cache       *Cache
	config      *Config
	upServers   []string
	upServerMap map[string][]string
	handlers    []HandleFunc
}

// NewClient returns a new dns client.
func NewClient(dialer proxy.Dialer, config *Config) (*Client, error) {
	c := &Client{
		dialer:      dialer,
		cache:       NewCache(),
		config:      config,
		upServers:   config.Servers,
		upServerMap: make(map[string][]string),
	}

	// custom records
	for _, record := range config.Records {
		c.AddRecord(record)
	}

	return c, nil
}

// Exchange handles request message and returns response message.
// reqBytes = reqLen + reqMsg
func (c *Client) Exchange(reqBytes []byte, clientAddr string, preferTCP bool) ([]byte, error) {
	req, err := UnmarshalMessage(reqBytes[2:])
	if err != nil {
		return nil, err
	}

	if req.Question.QTYPE == QTypeA || req.Question.QTYPE == QTypeAAAA {
		v := c.cache.Get(getKey(req.Question))
		if v != nil {
			binary.BigEndian.PutUint16(v[2:4], req.ID)
			log.F("[dns] %s <-> cache, type: %d, %s",
				clientAddr, req.Question.QTYPE, req.Question.QNAME)

			return v, nil
		}
	}

	dnsServer, network, respBytes, err := c.exchange(req.Question.QNAME, reqBytes, preferTCP)
	if err != nil {
		return nil, err
	}

	if req.Question.QTYPE != QTypeA && req.Question.QTYPE != QTypeAAAA {
		log.F("[dns] %s <-> %s(%s), type: %d, %s",
			clientAddr, dnsServer, network, req.Question.QTYPE, req.Question.QNAME)
		return respBytes, nil
	}

	resp, err := UnmarshalMessage(respBytes[2:])
	if err != nil {
		return respBytes, err
	}

	ttl := c.config.MinTTL
	ips := []string{}
	for _, answer := range resp.Answers {
		if answer.TYPE == QTypeA || answer.TYPE == QTypeAAAA {
			for _, h := range c.handlers {
				h(resp.Question.QNAME, answer.IP)
			}
			if answer.IP != "" {
				ips = append(ips, answer.IP)
			}
			if answer.TTL != 0 {
				ttl = int(answer.TTL)
			}
		}
	}

	if ttl > c.config.MaxTTL {
		ttl = c.config.MaxTTL
	} else if ttl < c.config.MinTTL {
		ttl = c.config.MinTTL
	}

	// add to cache only when there's a valid ip address
	if len(ips) != 0 && ttl > 0 {
		c.cache.Put(getKey(resp.Question), respBytes, ttl)
	}

	log.F("[dns] %s <-> %s(%s), type: %d, %s: %s",
		clientAddr, dnsServer, network, resp.Question.QTYPE, resp.Question.QNAME, strings.Join(ips, ","))

	return respBytes, nil
}

// exchange choose a upstream dns server based on qname, communicate with it on the network.
func (c *Client) exchange(qname string, reqBytes []byte, preferTCP bool) (server, network string, respBytes []byte, err error) {
	// use tcp to connect upstream server default
	network = "tcp"
	dialer := c.dialer.NextDialer(qname + ":53")

	// if we are resolving the dialer's domain, then use Direct to avoid denpency loop
	// TODO: dialer.Addr() == "reject", tricky
	if strings.Contains(dialer.Addr(), qname) || dialer.Addr() == "REJECT" {
		dialer = proxy.Default
	}

	// If client uses udp and no forwarders specified, use udp
	// TODO: dialer.Addr() == "DIRECT", tricky
	if !preferTCP && !c.config.AlwaysTCP && dialer.Addr() == "DIRECT" {
		network = "udp"
	}

	servers := c.GetServers(qname)
	for _, server = range servers {
		var rc net.Conn
		rc, err = dialer.Dial(network, server)
		if err != nil {
			log.F("[dns] failed to connect to server %v: %v", server, err)
			continue
		}
		defer rc.Close()

		// TODO: support timeout setting for different upstream server
		rc.SetDeadline(time.Now().Add(time.Duration(c.config.Timeout) * time.Second))

		switch network {
		case "tcp":
			respBytes, err = c.exchangeTCP(rc, reqBytes)
		case "udp":
			respBytes, err = c.exchangeUDP(rc, reqBytes)
		}

		if err == nil {
			break
		}

		log.F("[dns] failed to exchange with server %v: %v", server, err)
	}

	return server, network, respBytes, err
}

// exchangeTCP exchange with server over tcp.
func (c *Client) exchangeTCP(rc net.Conn, reqBytes []byte) ([]byte, error) {
	if _, err := rc.Write(reqBytes); err != nil {
		log.F("[dns] failed to write req message: %v", err)
		return nil, err
	}

	var respLen uint16
	if err := binary.Read(rc, binary.BigEndian, &respLen); err != nil {
		log.F("[dns] failed to read response length: %v", err)
		return nil, err
	}

	respBytes := make([]byte, respLen+2)
	binary.BigEndian.PutUint16(respBytes[:2], respLen)

	_, err := io.ReadFull(rc, respBytes[2:])
	if err != nil {
		log.F("[dns] error in read respMsg %s\n", err)
		return nil, err
	}

	return respBytes, nil
}

// exchangeUDP exchange with server over udp.
func (c *Client) exchangeUDP(rc net.Conn, reqBytes []byte) ([]byte, error) {
	if _, err := rc.Write(reqBytes[2:]); err != nil {
		log.F("[dns] failed to write req message: %v", err)
		return nil, err
	}

	reqBytes = make([]byte, 2+UDPMaxLen)
	n, err := rc.Read(reqBytes[2:])
	if err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint16(reqBytes[:2], uint16(n))

	return reqBytes[:2+n], nil
}

// SetServers sets upstream dns servers for the given domain.
func (c *Client) SetServers(domain string, servers ...string) {
	c.upServerMap[domain] = append(c.upServerMap[domain], servers...)
}

// GetServers gets upstream dns servers for the given domain
func (c *Client) GetServers(domain string) []string {
	domainParts := strings.Split(domain, ".")
	length := len(domainParts)
	for i := length - 1; i >= 0; i-- {
		domain := strings.Join(domainParts[i:length], ".")

		if servers, ok := c.upServerMap[domain]; ok {
			return servers
		}
	}

	return c.upServers
}

// AddHandler adds a custom handler to handle the resolved result (A and AAAA).
func (c *Client) AddHandler(h HandleFunc) {
	c.handlers = append(c.handlers, h)
}

// AddRecord adds custom record to dns cache, format:
// www.example.com/1.2.3.4 or www.example.com/2606:2800:220:1:248:1893:25c8:1946
func (c *Client) AddRecord(record string) error {
	r := strings.Split(record, "/")
	domain, ip := r[0], r[1]
	m, err := c.GenResponse(domain, ip)
	if err != nil {
		return err
	}

	b, _ := m.Marshal()

	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint16(len(b)))
	buf.Write(b)

	c.cache.Put(getKey(m.Question), buf.Bytes(), LongTTL)

	return nil
}

// GenResponse generates a dns response message for the given domain and ip address.
func (c *Client) GenResponse(domain string, ip string) (*Message, error) {
	ipb := net.ParseIP(ip)
	if ipb == nil {
		return nil, errors.New("GenResponse: invalid ip format")
	}

	var rdata []byte
	var qtype, rdlen uint16
	if rdata = ipb.To4(); rdata != nil {
		qtype = QTypeA
		rdlen = net.IPv4len
	} else {
		qtype = QTypeAAAA
		rdlen = net.IPv6len
		rdata = ipb
	}

	m := NewMessage(0, Response)
	m.SetQuestion(NewQuestion(qtype, domain))
	rr := &RR{NAME: domain, TYPE: qtype, CLASS: ClassINET,
		TTL: uint32(c.config.MinTTL), RDLENGTH: rdlen, RDATA: rdata}
	m.AddAnswer(rr)

	return m, nil
}

func getKey(q *Question) string {
	qtype := ""
	switch q.QTYPE {
	case QTypeA:
		qtype = "A"
	case QTypeAAAA:
		qtype = "AAAA"
	}
	return q.QNAME + "/" + qtype
}
