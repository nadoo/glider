package vmess

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// VMess struct.
type VMess struct {
	dialer proxy.Dialer
	addr   string

	uuid     string
	aead     bool
	alterID  int
	security string

	client *Client
}

func init() {
	proxy.RegisterDialer("vmess", NewVMessDialer)
}

// decodeVMessURL decodes BASE64-encoded vmess URL and converts it to standard format
// Input can be: vmess://base64EncodedJSON or vmess://standard-url-format
// BASE64 format typically comes from subscription services
func decodeVMessURL(s string) string {
	// Extract the part after vmess://
	if !strings.HasPrefix(s, "vmess://") {
		return s
	}

	urlPart := s[8:] // Remove "vmess://"

	// Try to decode as BASE64
	decoded, err := base64.StdEncoding.DecodeString(urlPart)
	if err != nil {
		// Not BASE64, return original
		return s
	}

	// Try to unmarshal as JSON
	var config map[string]interface{}
	err = json.Unmarshal(decoded, &config)
	if err != nil {
		// Not valid JSON, return original
		return s
	}

	// Extract configuration from JSON
	// Expected fields: id (uuid), add (address), port, aid (alterID), security (default ""), etc.
	uuid, ok := config["id"].(string)
	if !ok || uuid == "" {
		return s
	}

	addr, ok := config["add"].(string)
	if !ok || addr == "" {
		return s
	}

	port, ok := config["port"].(string)
	if !ok {
		// Try to convert from number to string
		if portNum, ok := config["port"].(float64); ok {
			port = strconv.FormatFloat(portNum, 'f', 0, 64)
		} else {
			return s
		}
	}

	// Get alterID (default "0")
	alterID := "0"
	if aid, ok := config["aid"].(string); ok {
		alterID = aid
	} else if aidNum, ok := config["aid"].(float64); ok {
		alterID = strconv.FormatFloat(aidNum, 'f', 0, 64)
	}

	// Get security/cipher (default empty string)
	security := ""
	if sec, ok := config["scy"].(string); ok {
		security = sec
	} else if sec, ok := config["security"].(string); ok {
		security = sec
	}

	// Reconstruct as standard vmess URL format
	// Format: vmess://[security:]uuid@host:port[?alterID=num]
	var standardURL string
	if security != "" {
		standardURL = "vmess://" + security + ":" + uuid + "@" + addr + ":" + port + "?alterID=" + alterID
	} else {
		standardURL = "vmess://" + uuid + "@" + addr + ":" + port + "?alterID=" + alterID
	}

	return standardURL
}

// NewVMess returns a vmess proxy.
func NewVMess(s string, d proxy.Dialer) (*VMess, error) {
	// Handle BASE64 encoded vmess URL
	s = decodeVMessURL(s)

	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	addr := u.Host
	security := u.User.Username()
	uuid, ok := u.User.Password()
	if !ok {
		// no security type specified, vmess://uuid@server
		uuid = security
		security = ""
	}

	query := u.Query()
	aid := query.Get("alterID")
	if aid == "" {
		aid = "0"
	}

	alterID, err := strconv.ParseUint(aid, 10, 32)
	if err != nil {
		log.F("parse alterID err: %s", err)
		return nil, err
	}

	aead := alterID == 0
	client, err := NewClient(uuid, security, int(alterID), aead)
	if err != nil {
		log.F("create vmess client err: %s", err)
		return nil, err
	}

	p := &VMess{
		dialer:   d,
		addr:     addr,
		uuid:     uuid,
		alterID:  int(alterID),
		security: security,
		client:   client,
	}

	return p, nil
}

// NewVMessDialer returns a vmess proxy dialer.
func NewVMessDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewVMess(s, dialer)
}

// Addr returns forwarder's address.
func (s *VMess) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *VMess) Dial(network, addr string) (net.Conn, error) {
	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		return nil, err
	}

	return s.client.NewConn(rc, addr, CmdTCP)
}

// DialUDP connects to the given address via the proxy.
func (s *VMess) DialUDP(network, addr string) (net.PacketConn, error) {
	tgtAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.F("[vmess] error in ResolveUDPAddr: %v", err)
		return nil, err
	}

	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		return nil, err
	}
	rc, err = s.client.NewConn(rc, addr, CmdUDP)
	if err != nil {
		return nil, err
	}

	return NewPktConn(rc, tgtAddr), err
}

func init() {
	proxy.AddUsage("vmess", `
VMess scheme:
  vmess://[security:]uuid@host:port[?alterID=num]
    if alterID=0 or not set, VMessAEAD will be enabled
  
  Available security for vmess:
    zero, none, aes-128-gcm, chacha20-poly1305
`)
}
