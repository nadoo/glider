package main

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	shadowsocksr "github.com/sun8911879/shadowsocksR"
	"github.com/sun8911879/shadowsocksR/obfs"
	"github.com/sun8911879/shadowsocksR/protocol"
	"github.com/sun8911879/shadowsocksR/ssr"
)

// SSR .
type SSR struct {
	dialer Dialer
	addr   string

	EncryptMethod   string
	EncryptPassword string
	Obfs            string
	ObfsParam       string
	ObfsData        interface{}
	Protocol        string
	ProtocolParam   string
	ProtocolData    interface{}
}

// NewSSR returns a shadowsocksr proxy, ssr://method:pass@host:port/rawQuery
func NewSSR(addr, method, pass, rawQuery string, dialer Dialer) (*SSR, error) {
	s := &SSR{
		dialer:          dialer,
		addr:            addr,
		EncryptMethod:   method,
		EncryptPassword: pass,
	}

	p, _ := url.ParseQuery(rawQuery)
	if v, ok := p["protocol"]; ok {
		s.Protocol = v[0]
	}
	if v, ok := p["protocol_param"]; ok {
		s.ProtocolParam = v[0]
	}
	if v, ok := p["obfs"]; ok {
		s.Obfs = v[0]
	}
	if v, ok := p["obfs_param"]; ok {
		s.ObfsParam = v[0]
	}

	return s, nil
}

// Addr returns forwarder's address
func (s *SSR) Addr() string { return s.addr }

// NextDialer returns the next dialer
func (s *SSR) NextDialer(dstAddr string) Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial connects to the address addr on the network net via the proxy.
func (s *SSR) Dial(network, addr string) (net.Conn, error) {
	target := ParseAddr(addr)
	if target == nil {
		return nil, errors.New("proxy-ssr unable to parse address: " + addr)
	}

	cipher, err := shadowsocksr.NewStreamCipher(s.EncryptMethod, s.EncryptPassword)
	if err != nil {
		return nil, err
	}

	c, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		logf("proxy-ssr dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	ssrconn := shadowsocksr.NewSSTCPConn(c, cipher)
	if ssrconn.Conn == nil || ssrconn.RemoteAddr() == nil {
		return nil, errors.New("proxy-ssr nil connection")
	}

	// should initialize obfs/protocol now
	rs := strings.Split(ssrconn.RemoteAddr().String(), ":")
	port, _ := strconv.Atoi(rs[1])

	ssrconn.IObfs = obfs.NewObfs(s.Obfs)
	obfsServerInfo := &ssr.ServerInfoForObfs{
		Host:   rs[0],
		Port:   uint16(port),
		TcpMss: 1460,
		Param:  s.ObfsParam,
	}
	ssrconn.IObfs.SetServerInfo(obfsServerInfo)
	ssrconn.IProtocol = protocol.NewProtocol(s.Protocol)
	protocolServerInfo := &ssr.ServerInfoForObfs{
		Host:   rs[0],
		Port:   uint16(port),
		TcpMss: 1460,
		Param:  s.ProtocolParam,
	}
	ssrconn.IProtocol.SetServerInfo(protocolServerInfo)

	if s.ObfsData == nil {
		s.ObfsData = ssrconn.IObfs.GetData()
	}
	ssrconn.IObfs.SetData(s.ObfsData)

	if s.ProtocolData == nil {
		s.ProtocolData = ssrconn.IProtocol.GetData()
	}
	ssrconn.IProtocol.SetData(s.ProtocolData)

	if _, err := ssrconn.Write(target); err != nil {
		ssrconn.Close()
		return nil, err
	}

	return ssrconn, err
}

// DialUDP connects to the given address via the proxy.
func (s *SSR) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("proxy-ssr udp not supported now")
}
