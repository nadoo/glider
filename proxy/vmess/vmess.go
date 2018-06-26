package vmess

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"

	"v2ray.com/core"
	"v2ray.com/core/app/dispatcher"
	"v2ray.com/core/app/proxyman"
	v2net "v2ray.com/core/common/net"
	"v2ray.com/core/common/protocol"
	"v2ray.com/core/common/serial"
	"v2ray.com/core/proxy/vmess"
	"v2ray.com/core/proxy/vmess/outbound"
	"v2ray.com/core/transport/internet"
	"v2ray.com/core/transport/internet/tls"

	_ "v2ray.com/core/app/proxyman/outbound"
	_ "v2ray.com/core/transport/internet/tcp"
)

// VMess .
type VMess struct {
	dialer proxy.Dialer
	addr   string

	uuid     string
	alertID  uint32
	network  string
	security string

	config   *core.Config
	instance *core.Instance
}

func init() {
	proxy.RegisterDialer("vmess", NewVMessDialer)
}

// NewVMess returns a vmess proxy.
func NewVMess(s string, dialer proxy.Dialer) (*VMess, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	addr := u.Host
	host := u.Hostname()
	port, err := strconv.ParseUint(u.Port(), 10, 32)
	if err != nil {
		log.F("parse port err: %s", err)
		return nil, err
	}

	var uuid, aid string
	if u.User != nil {
		uuid = u.User.Username()
		aid, _ = u.User.Password()
	}

	alertID, err := strconv.ParseUint(aid, 10, 32)
	if err != nil {
		log.F("parse alertID err: %s", err)
		return nil, err
	}

	config := &core.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
		},
		Outbound: []*core.OutboundHandlerConfig{{
			ProxySettings: serial.ToTypedMessage(&outbound.Config{
				Receiver: []*protocol.ServerEndpoint{
					{
						Address: v2net.NewIPOrDomain(v2net.ParseAddress(host)),
						Port:    uint32(port),
						User: []*protocol.User{
							{
								Account: serial.ToTypedMessage(&vmess.Account{
									Id:      uuid,
									AlterId: uint32(alertID),
									SecuritySettings: &protocol.SecurityConfig{
										Type: protocol.SecurityType_NONE,
									},
								}),
							},
						},
					},
				},
			}),
			SenderSettings: serial.ToTypedMessage(&proxyman.SenderConfig{
				StreamSettings: &internet.StreamConfig{
					Protocol:     internet.TransportProtocol_TCP,
					SecurityType: serial.GetMessageType(&tls.Config{}),
					SecuritySettings: []*serial.TypedMessage{
						serial.ToTypedMessage(&tls.Config{
							AllowInsecure: true,
						}),
					},
				},
			})},
		},
	}

	v, err := core.New(config)
	if err != nil {
		log.Fatal("Failed to create V: ", err.Error())
	}

	p := &VMess{
		dialer: dialer,
		addr:   addr,

		uuid:     uuid,
		alertID:  uint32(alertID),
		network:  "tcp",
		security: "tls",

		config:   config,
		instance: v,
	}

	return p, nil
}

// NewVMessDialer returns a vmess proxy dialer.
func NewVMessDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewVMess(s, dialer)
}

// Addr returns forwarder's address
func (s *VMess) Addr() string { return s.addr }

// NextDialer returns the next dialer
func (s *VMess) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial connects to the address addr on the network net via the proxy.
func (s *VMess) Dial(network, addr string) (net.Conn, error) {

	// c, err := s.dialer.Dial("tcp", s.addr)

	d := strings.Split(addr, ":")
	host, portStr := d[0], d[1]
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		log.F("parse portStr err: %s", err)
		return nil, err
	}

	// TODO: does not support upstream dialer now
	c, err := core.Dial(context.Background(), s.instance, v2net.TCPDestination(v2net.ParseAddress(host), v2net.Port(port)))
	if err != nil {
		log.F("proxy-vmess dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	return c, err

}

// DialUDP connects to the given address via the proxy.
func (s *VMess) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("vmess client does not support udp now")
}
