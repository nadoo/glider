package dns

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
)

// AnswerHandler function handles the dns TypeA or TypeAAAA answer.
type AnswerHandler func(domain, ip string) error

// Config for dns.
type Config struct {
	Servers   []string
	Timeout   int
	MaxTTL    int
	MinTTL    int
	Records   []string
	AlwaysTCP bool
	CacheSize int
}

// Client is a dns client struct.
type Client struct {
	proxy       proxy.Proxy
	cache       *LruCache
	config      *Config
	upStream    *UPStream
	upStreamMap map[string]*UPStream
	handlers    []AnswerHandler
}

// NewClient returns a new dns client.
func NewClient(proxy proxy.Proxy, config *Config) (*Client, error) {
	c := &Client{
		proxy:       proxy,
		cache:       NewLruCache(config.CacheSize),
		config:      config,
		upStream:    NewUPStream(config.Servers),
		upStreamMap: make(map[string]*UPStream),
	}

	// custom records
	for _, record := range config.Records {
		c.AddRecord(record)
	}

	return c, nil
}

// Exchange handles request message and returns response message.
// NOTE: reqBytes = reqLen + reqMsg.
func (c *Client) Exchange(reqBytes []byte, clientAddr string, preferTCP bool) ([]byte, error) {
	req, err := UnmarshalMessage(reqBytes[2:])
	if err != nil {
		return nil, err
	}

	if req.Question.QTYPE == QTypeA || req.Question.QTYPE == QTypeAAAA {
		v, _ := c.cache.Get(qKey(req.Question))
		if len(v) > 4 {
			v = valCopy(v)
			binary.BigEndian.PutUint16(v[2:4], req.ID)
			log.F("[dns] %s <-> cache, type: %d, %s",
				clientAddr, req.Question.QTYPE, req.Question.QNAME)

			return v, nil
		}
	}

	dnsServer, network, dialerAddr, respBytes, err := c.exchange(req.Question.QNAME, reqBytes, preferTCP)
	if err != nil {
		return nil, err
	}

	if req.Question.QTYPE != QTypeA && req.Question.QTYPE != QTypeAAAA {
		log.F("[dns] %s <-> %s(%s) via %s, type: %d, %s",
			clientAddr, dnsServer, network, dialerAddr, req.Question.QTYPE, req.Question.QNAME)
		return respBytes, nil
	}

	resp, err := UnmarshalMessage(respBytes[2:])
	if err != nil {
		return respBytes, err
	}

	ips, ttl := c.extractAnswer(resp)

	// add to cache only when there's a valid ip address
	if len(ips) != 0 && ttl > 0 {
		c.cache.Set(qKey(resp.Question), valCopy(respBytes), ttl)
	}

	log.F("[dns] %s <-> %s(%s) via %s, type: %d, %s: %s",
		clientAddr, dnsServer, network, dialerAddr, resp.Question.QTYPE, resp.Question.QNAME, strings.Join(ips, ","))

	return respBytes, nil
}

func (c *Client) extractAnswer(resp *Message) ([]string, int) {
	var ips []string
	ttl := c.config.MinTTL
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

	return ips, ttl
}

// exchange choose a upstream dns server based on qname, communicate with it on the network.
func (c *Client) exchange(qname string, reqBytes []byte, preferTCP bool) (
	server, network, dialerAddr string, respBytes []byte, err error) {

	// use tcp to connect upstream server default
	network = "tcp"
	dialer := c.proxy.NextDialer(qname + ":53")

	// if we are resolving the dialer's domain, then use Direct to avoid denpency loop
	// TODO: dialer.Addr() == "REJECT", tricky
	if strings.Contains(dialer.Addr(), qname) || dialer.Addr() == "REJECT" {
		dialer = proxy.Default
	}

	// If client uses udp and no forwarders specified, use udp
	// TODO: dialer.Addr() == "DIRECT", tricky
	if !preferTCP && !c.config.AlwaysTCP && dialer.Addr() == "DIRECT" {
		network = "udp"
	}

	ups := c.UpStream(qname)
	server = ups.Server()
	for i := 0; i < ups.Len(); i++ {
		var rc net.Conn
		rc, err = dialer.Dial(network, server)
		if err != nil {
			newServer := ups.SwitchIf(server)
			log.F("[dns] error in resolving %s, failed to connect to server %v via %s: %v, next server: %s",
				qname, server, dialer.Addr(), err, newServer)
			server = newServer
			continue
		}
		defer rc.Close()

		// TODO: support timeout setting for different upstream server
		if c.config.Timeout > 0 {
			rc.SetDeadline(time.Now().Add(time.Duration(c.config.Timeout) * time.Second))
		}

		switch network {
		case "tcp":
			respBytes, err = c.exchangeTCP(rc, reqBytes)
		case "udp":
			respBytes, err = c.exchangeUDP(rc, reqBytes)
		}

		if err == nil {
			break
		}

		newServer := ups.SwitchIf(server)
		log.F("[dns] error in resolving %s, failed to exchange with server %v via %s: %v, next server: %s",
			qname, server, dialer.Addr(), err, newServer)

		server = newServer
	}

	// if all dns upstreams failed, then maybe the forwarder is not available.
	if err != nil {
		c.proxy.Record(dialer, false)
	}

	return server, network, dialer.Addr(), respBytes, err
}

// exchangeTCP exchange with server over tcp.
func (c *Client) exchangeTCP(rc net.Conn, reqBytes []byte) ([]byte, error) {
	if _, err := rc.Write(reqBytes); err != nil {
		return nil, err
	}

	var respLen uint16
	if err := binary.Read(rc, binary.BigEndian, &respLen); err != nil {
		return nil, err
	}

	respBytes := pool.GetBuffer(int(respLen) + 2)
	binary.BigEndian.PutUint16(respBytes[:2], respLen)

	_, err := io.ReadFull(rc, respBytes[2:])
	if err != nil {
		return nil, err
	}

	return respBytes, nil
}

// exchangeUDP exchange with server over udp.
func (c *Client) exchangeUDP(rc net.Conn, reqBytes []byte) ([]byte, error) {
	if _, err := rc.Write(reqBytes[2:]); err != nil {
		return nil, err
	}

	respBytes := pool.GetBuffer(UDPMaxLen)
	n, err := rc.Read(respBytes[2:])
	if err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint16(respBytes[:2], uint16(n))

	return respBytes[:2+n], nil
}

// SetServers sets upstream dns servers for the given domain.
func (c *Client) SetServers(domain string, servers []string) {
	c.upStreamMap[strings.ToLower(domain)] = NewUPStream(servers)
}

// UpStream returns upstream dns server for the given domain.
func (c *Client) UpStream(domain string) *UPStream {
	domain = strings.ToLower(domain)
	for i := len(domain); i != -1; {
		i = strings.LastIndexByte(domain[:i], '.')
		if upstream, ok := c.upStreamMap[domain[i+1:]]; ok {
			return upstream
		}
	}
	return c.upStream
}

// AddHandler adds a custom handler to handle the resolved result (A and AAAA).
func (c *Client) AddHandler(h AnswerHandler) {
	c.handlers = append(c.handlers, h)
}

// AddRecord adds custom record to dns cache, format:
// www.example.com/1.2.3.4 or www.example.com/2606:2800:220:1:248:1893:25c8:1946
func (c *Client) AddRecord(record string) error {
	r := strings.Split(record, "/")
	domain, ip := r[0], r[1]
	m, err := c.MakeResponse(domain, ip)
	if err != nil {
		return err
	}

	wb := pool.GetWriteBuffer()
	defer pool.PutWriteBuffer(wb)

	wb.Write([]byte{0, 0})

	n, err := m.MarshalTo(wb)
	if err != nil {
		return err
	}

	binary.BigEndian.PutUint16(wb.Bytes()[:2], uint16(n))
	c.cache.Set(qKey(m.Question), wb.Bytes(), 0)

	return nil
}

// MakeResponse makes a dns response message for the given domain and ip address.
func (c *Client) MakeResponse(domain string, ip string) (*Message, error) {
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

func qKey(q *Question) string {
	switch q.QTYPE {
	case QTypeA:
		return q.QNAME + "/4"
	case QTypeAAAA:
		return q.QNAME + "/6"
	}
	return q.QNAME
}

func valCopy(v []byte) (b []byte) {
	if v != nil {
		b = pool.GetBuffer(len(v))
		copy(b, v)
	}
	return
}
