package dns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// DefaultTTL is default ttl in seconds
const DefaultTTL = 600

// HandleFunc function handles the dns TypeA or TypeAAAA answer
type HandleFunc func(Domain, ip string) error

// Client is a dns client struct
type Client struct {
	dialer      proxy.Dialer
	cache       *Cache
	upServers   []string
	upServerMap map[string][]string
	handlers    []HandleFunc
}

// NewClient returns a new dns client
func NewClient(dialer proxy.Dialer, upServers []string) (*Client, error) {
	c := &Client{
		dialer:      dialer,
		cache:       NewCache(),
		upServers:   upServers,
		upServerMap: make(map[string][]string),
	}

	return c, nil
}

// Exchange handles request msg and returns response msg
// reqBytes = reqLen + reqMsg
func (c *Client) Exchange(reqBytes []byte, clientAddr string) ([]byte, error) {
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

	dnsServer := c.GetServer(req.Question.QNAME)
	rc, err := c.dialer.NextDialer(req.Question.QNAME+":53").Dial("tcp", dnsServer)
	if err != nil {
		log.F("[dns] failed to connect to server %v: %v", dnsServer, err)
		return nil, err
	}
	defer rc.Close()

	if err = binary.Write(rc, binary.BigEndian, reqBytes); err != nil {
		log.F("[dns] failed to write req message: %v", err)
		return nil, err
	}

	var respLen uint16
	if err = binary.Read(rc, binary.BigEndian, &respLen); err != nil {
		log.F("[dns] failed to read response length: %v", err)
		return nil, err
	}

	respBytes := make([]byte, respLen+2)
	binary.BigEndian.PutUint16(respBytes[:2], respLen)

	_, err = io.ReadFull(rc, respBytes[2:])
	if err != nil {
		log.F("[dns] error in read respMsg %s\n", err)
		return nil, err
	}

	if req.Question.QTYPE != QTypeA && req.Question.QTYPE != QTypeAAAA {
		log.F("[dns] %s <-> %s, type: %d, %s",
			clientAddr, dnsServer, req.Question.QTYPE, req.Question.QNAME)
		return respBytes, nil
	}

	resp, err := UnmarshalMessage(respBytes[2:])
	if err != nil {
		return respBytes, err
	}

	ttl := DefaultTTL
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

	// add to cache
	c.cache.Put(getKey(resp.Question), respBytes, ttl)

	log.F("[dns] %s <-> %s, type: %d, %s: %s",
		clientAddr, dnsServer, resp.Question.QTYPE, resp.Question.QNAME, strings.Join(ips, ","))

	return respBytes, nil
}

// SetServer .
func (c *Client) SetServer(domain string, servers ...string) {
	c.upServerMap[domain] = append(c.upServerMap[domain], servers...)
}

// GetServer .
func (c *Client) GetServer(domain string) string {
	domainParts := strings.Split(domain, ".")
	length := len(domainParts)
	for i := length - 2; i >= 0; i-- {
		domain := strings.Join(domainParts[i:length], ".")

		if servers, ok := c.upServerMap[domain]; ok {
			return servers[0]
		}
	}

	// TODO:
	return c.upServers[0]
}

// AddHandler .
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

	c.cache.Put(getKey(m.Question), buf.Bytes(), HundredYears)

	return nil
}

// GenResponse .
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
	rr := &RR{NAME: domain, TYPE: qtype, CLASS: CLASSIN,
		RDLENGTH: rdlen, RDATA: rdata}
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
