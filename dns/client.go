package dns

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
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
	httpClient *http.Client
}

// NewClient returns a new dns client.
func NewClient(proxy proxy.Proxy, config *Config) (*Client, error) {
	c := &Client{
		proxy:       proxy,
		cache:       NewLruCache(config.CacheSize),
		config:      config,
		upStream:    NewUPStream(config.Servers),
		upStreamMap: make(map[string]*UPStream),
		httpClient: &http.Client{
		},
	}

	// custom records
	for _, record := range config.Records {
		c.AddRecord(record)
	}

	return c, nil
}

// Exchange handles request message and returns response message.
// TODO: optimize it
func (c *Client) Exchange(reqBytes []byte, clientAddr string, preferTCP bool) ([]byte, error) {
	req, err := UnmarshalMessage(reqBytes)
	if err != nil {
		return nil, err
	}

	if req.Question.QTYPE == QTypeA || req.Question.QTYPE == QTypeAAAA {
		if v, expired := c.cache.Get(qKey(req.Question)); len(v) > 2 {
			v = valCopy(v)
			binary.BigEndian.PutUint16(v[:2], req.ID)

			log.F("[dns] %s <-> cache, type: %d, %s",
				clientAddr, req.Question.QTYPE, req.Question.QNAME)

			if expired { // update cache
				go func(qname string, reqBytes []byte, preferTCP bool) {
					defer pool.PutBuffer(reqBytes)
					if dnsServer, network, dialerAddr, respBytes, err := c.exchange(qname, reqBytes, preferTCP); err == nil {
						c.handleAnswer(respBytes, "cache", dnsServer, network, dialerAddr)
					}
				}(req.Question.QNAME, valCopy(reqBytes), preferTCP)
			}
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

	err = c.handleAnswer(respBytes, clientAddr, dnsServer, network, dialerAddr)
	return respBytes, err
}

func (c *Client) handleAnswer(respBytes []byte, clientAddr, dnsServer, network, dialerAddr string) error {
	resp, err := UnmarshalMessage(respBytes)
	if err != nil {
		return err
	}

	ips, ttl := c.extractAnswer(resp)
	if len(ips) != 0 && ttl > 0 {
		c.cache.Set(qKey(resp.Question), valCopy(respBytes), ttl)
	}

	log.F("[dns] %s <-> %s(%s) via %s, type: %d, %s: %s",
		clientAddr, dnsServer, network, dialerAddr, resp.Question.QTYPE, resp.Question.QNAME, strings.Join(ips, ","))

	return nil
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
	ups := c.UpStream(qname)
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
	//init conn and option
	var rc net.Conn
	var op string
	for i := 0; i < ups.Len(); i++ {
		u, err := url.Parse(ups.Server())
		if err!=nil{
			server=ups.Server()
			op=network
		}else{
			server=u.Host
			op=u.Scheme
		}
		//if not set option use network else use special option
		switch op{
		case "tcp":
			network = "tcp"
			rc, err = dialer.Dial(network, server)
		case "udp":
			network = "udp"
			rc, err = dialer.Dial(network, server)
		case "dot":
			rc,err=tls.Dial("tcp",server,&tls.Config{InsecureSkipVerify: false,})
		case "doh":
			net.DefaultResolver=&net.Resolver{}
		default:
			break
		}
		if err != nil {
			newServer := ups.SwitchIf(server)
			log.F("[dns] error in resolving %s, failed to connect to server %v via %s: %v, next server: %s",
				qname, server, dialer.Addr(), err, newServer)
			server = newServer
			continue
		}
		//TODO: if we use DOH (network=="doh") we don't need close connection
		if network!="doh"{
			defer rc.Close()
		}

		// TODO: support timeout setting for different upstream server
		if c.config.Timeout > 0 && network!="doh" {
			rc.SetDeadline(time.Now().Add(time.Duration(c.config.Timeout) * time.Second))
		}

		switch op {
		case "tcp","dot":
			respBytes, err = c.exchangeTCP(rc, reqBytes)
		case "udp":
			respBytes, err = c.exchangeUDP(rc, reqBytes)
		case "doh":
			respBytes, err = c.exchangeHTTPS(server, reqBytes)
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

	return server, op, dialer.Addr(), respBytes, err
}
//exchangeHTTP exchange with server over https
func (c*Client) exchangeHTTPS(server string,reqBytes[]byte)(body[]byte,err error){
	query := strings.Replace(base64.URLEncoding.EncodeToString(reqBytes), "=", "", -1)
	urls := "https://" + server + "/dns-query?dns=" + query
	res, err := c.httpClient.Get(urls)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return
}
// exchangeTCP exchange with server over tcp.
func (c *Client) exchangeTCP(rc net.Conn, reqBytes []byte) ([]byte, error) {
	lenBuf := pool.GetBuffer(2)
	defer pool.PutBuffer(lenBuf)

	binary.BigEndian.PutUint16(lenBuf, uint16(len(reqBytes)))
	if _, err := (&net.Buffers{lenBuf, reqBytes}).WriteTo(rc); err != nil {
		return nil, err
	}

	var respLen uint16
	if err := binary.Read(rc, binary.BigEndian, &respLen); err != nil {
		return nil, err
	}

	respBytes := pool.GetBuffer(int(respLen))
	_, err := io.ReadFull(rc, respBytes)
	if err != nil {
		return nil, err
	}

	return respBytes, nil
}

// exchangeUDP exchange with server over udp.
func (c *Client) exchangeUDP(rc net.Conn, reqBytes []byte) ([]byte, error) {
	if _, err := rc.Write(reqBytes); err != nil {
		return nil, err
	}

	respBytes := pool.GetBuffer(UDPMaxLen)
	n, err := rc.Read(respBytes)
	if err != nil {
		return nil, err
	}

	return respBytes[:n], nil
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

	wb := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(wb)

	_, err = m.MarshalTo(wb)
	if err != nil {
		return err
	}

	c.cache.Set(qKey(m.Question), valCopy(wb.Bytes()), 0)

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
	return q.QNAME + "/" + strconv.FormatUint(uint64(q.QTYPE), 10)
}

func valCopy(v []byte) (b []byte) {
	if v != nil {
		b = pool.GetBuffer(len(v))
		copy(b, v)
	}
	return
}
