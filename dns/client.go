package dns

import (
	"encoding/binary"
	"io"
	"strings"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// HandleFunc function handles the dns TypeA or TypeAAAA answer
type HandleFunc func(Domain, ip string) error

// Client is a dns client struct
type Client struct {
	dialer      proxy.Dialer
	UPServers   []string
	UPServerMap map[string][]string
	Handlers    []HandleFunc

	tcp bool
}

// NewClient returns a new dns client
func NewClient(dialer proxy.Dialer, upServers ...string) (*Client, error) {
	c := &Client{
		dialer:      dialer,
		UPServers:   upServers,
		UPServerMap: make(map[string][]string),
	}

	return c, nil
}

// Exchange handles request msg and returns response msg
// reqBytes = reqLen + reqMsg
func (c *Client) Exchange(reqBytes []byte, clientAddr string) (respBytes []byte, err error) {
	reqMsg := reqBytes[2:]

	reqM := NewMessage()
	err = UnmarshalMessage(reqMsg, reqM)
	if err != nil {
		return
	}

	if reqM.Question.QTYPE == QTypeA || reqM.Question.QTYPE == QTypeAAAA {
		// TODO: if query.QNAME in cache
		// get respMsg from cache
		// set msg id
		// return respMsg, nil
	}

	dnsServer := c.GetServer(reqM.Question.QNAME)
	rc, err := c.dialer.NextDialer(reqM.Question.QNAME+":53").Dial("tcp", dnsServer)
	if err != nil {
		log.F("[dns] failed to connect to server %v: %v", dnsServer, err)
		return
	}
	defer rc.Close()

	if err = binary.Write(rc, binary.BigEndian, reqBytes); err != nil {
		log.F("[dns] failed to write req message: %v", err)
		return
	}

	var respLen uint16
	if err = binary.Read(rc, binary.BigEndian, &respLen); err != nil {
		log.F("[dns] failed to read response length: %v", err)
		return
	}

	respBytes = make([]byte, respLen+2)
	binary.BigEndian.PutUint16(respBytes[:2], respLen)

	respMsg := respBytes[2:]
	_, err = io.ReadFull(rc, respMsg)
	if err != nil {
		log.F("[dns] error in read respMsg %s\n", err)
		return
	}

	if reqM.Question.QTYPE != QTypeA && reqM.Question.QTYPE != QTypeAAAA {
		return
	}

	respM := NewMessage()
	err = UnmarshalMessage(respMsg, respM)
	if err != nil {
		return
	}

	ip := ""
	for _, answer := range respM.Answers {
		if answer.TYPE == QTypeA {
			for _, h := range c.Handlers {
				h(reqM.Question.QNAME, answer.IP)
			}

			if answer.IP != "" {
				ip += answer.IP + ","
			}
		}

		log.F("rr: %+v", answer)
	}

	// add to cache

	log.F("[dns] %s <-> %s, type: %d, %s: %s",
		clientAddr, dnsServer, reqM.Question.QTYPE, reqM.Question.QNAME, ip)

	return
}

// SetServer .
func (c *Client) SetServer(domain string, servers ...string) {
	c.UPServerMap[domain] = append(c.UPServerMap[domain], servers...)
}

// GetServer .
func (c *Client) GetServer(domain string) string {
	domainParts := strings.Split(domain, ".")
	length := len(domainParts)
	for i := length - 2; i >= 0; i-- {
		domain := strings.Join(domainParts[i:length], ".")

		if servers, ok := c.UPServerMap[domain]; ok {
			return servers[0]
		}
	}

	// TODO:
	return c.UPServers[0]
}

// AddHandler .
func (c *Client) AddHandler(h HandleFunc) {
	c.Handlers = append(c.Handlers, h)
}
