package dhcpd

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/service"
)

func init() {
	service.Register("dhcpd", NewService)
	service.Register("dhcpd-failover", NewFailOverService)
}

type dhcpd struct {
	mu       sync.Mutex
	failover bool

	name   string
	pool   *Pool
	lease  time.Duration
	iface  *net.Interface
	server *server4.Server
}

// NewService returns a new dhcpd Service.
func NewService(args ...string) (service.Service, error) { return New(false, args...) }

// NewFailOverService returns a new dhcpd Service with failover mode on.
func NewFailOverService(args ...string) (service.Service, error) { return New(true, args...) }

// New returns a new dhcpd instance.
func New(failover bool, args ...string) (*dhcpd, error) {
	if len(args) < 4 {
		return nil, errors.New("not enough parameters, exiting")
	}

	iface, start, end, leaseMin := args[0], args[1], args[2], args[3]
	intf, ip, mask, err := ifaceAddr(iface)
	if err != nil {
		return nil, fmt.Errorf("get ip of interface '%s' error: %s", iface, err)
	}

	startIP, err := netip.ParseAddr(start)
	if err != nil {
		return nil, fmt.Errorf("startIP %s is not valid: %s", start, err)
	}

	endIP, err := netip.ParseAddr(end)
	if err != nil {
		return nil, fmt.Errorf("endIP %s is not valid: %s", end, err)
	}

	var lease = time.Hour * 12
	if i, err := strconv.Atoi(leaseMin); err == nil {
		lease = time.Duration(i) * time.Minute
	} else {
		return nil, fmt.Errorf("LEASE_MINUTES %s is not valid: %s", end, err)
	}

	pool, err := NewPool(lease, startIP, endIP)
	if err != nil {
		return nil, fmt.Errorf("error in pool init: %s", err)
	}

	// static ips
	for _, host := range args[4:] {
		if mac, ip, ok := strings.Cut(host, "="); ok {
			if mac, err := net.ParseMAC(mac); err == nil {
				if ip, err := netip.ParseAddr(ip); err == nil {
					pool.LeaseStaticIP(mac, ip)
				}
			}
		}
	}

	dhcpd := &dhcpd{
		name:     intf.Name,
		iface:    intf,
		pool:     pool,
		lease:    lease,
		failover: failover,
	}

	if dhcpd.server, err = server4.NewServer(
		iface, &net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: 67},
		dhcpd.handleDHCP(ip, mask, pool)); err != nil {
		return nil, fmt.Errorf("error in server creation: %s", err)
	}

	log.F("[dhcpd] Listening on interface %s(%s/%d.%d.%d.%d), failover mode: %t",
		iface, ip, mask[0], mask[1], mask[2], mask[3], dhcpd.isFailover())

	return dhcpd, nil
}

// Run runs the service.
func (d *dhcpd) Run() {
	if d.failover {
		d.setFailover(discovery(d.iface))
		go func() {
			for {
				d.setFailover(discovery(d.iface))
				time.Sleep(time.Second * 60)
			}
		}()
	}
	d.server.Serve()
}

func (d *dhcpd) handleDHCP(serverIP net.IP, mask net.IPMask, pool *Pool) server4.Handler {
	return func(conn net.PacketConn, peer net.Addr, m *dhcpv4.DHCPv4) {

		if d.isFailover() || bytes.Equal(d.iface.HardwareAddr, m.ClientHWAddr) {
			return
		}

		var reqIP netip.Addr
		var reqType, replyType dhcpv4.MessageType

		reqType = m.MessageType()
		log.F("[dpcpd] %s: %s from %v(%v)", d.name, reqType, m.ClientHWAddr, m.ClientIPAddr)

		switch reqType {
		case dhcpv4.MessageTypeDiscover:
			replyType = dhcpv4.MessageTypeOffer
		case dhcpv4.MessageTypeInform:
			replyType = dhcpv4.MessageTypeAck
		case dhcpv4.MessageTypeRequest:
			replyType = dhcpv4.MessageTypeAck
			if m.Options.Has(dhcpv4.OptionRequestedIPAddress) {
				reqIP, _ = netip.AddrFromSlice(m.Options.Get(dhcpv4.OptionRequestedIPAddress))
			} else {
				// client uses Unicast to renew ip address lease, just take client ip
				reqIP = netip.AddrFrom4([4]byte(m.ClientIPAddr.To4()))
			}
		case dhcpv4.MessageTypeRelease, dhcpv4.MessageTypeDecline:
			pool.ReleaseIP(m.ClientHWAddr)
			return
		default:
			log.F("[dpcpd] %s: can't handle type %v from %v", d.name, reqType, m.ClientHWAddr)
			return
		}

		replyIP, err := pool.LeaseIP(m.ClientHWAddr, reqIP)
		if err != nil {
			log.F("[dpcpd] %s: can not assign IP for %v, error: %s", d.name, m.ClientHWAddr, err)
			return
		}

		if reqType == dhcpv4.MessageTypeRequest && !reqIP.IsUnspecified() && reqIP != replyIP {
			replyType = dhcpv4.MessageTypeNak
		}

		resp, err := dhcpv4.NewReplyFromRequest(m,
			dhcpv4.WithMessageType(replyType),
			dhcpv4.WithNetmask(mask),
			dhcpv4.WithYourIP(replyIP.AsSlice()),
			dhcpv4.WithRouter(serverIP),
			dhcpv4.WithDNS(serverIP),
			dhcpv4.WithServerIP(serverIP), //
			// RFC 2131, Section 4.3.1. IP lease time: MUST
			dhcpv4.WithOption(dhcpv4.OptIPAddressLeaseTime(d.lease)),
			// RFC 2131, Section 4.3.1. Server Identifier: MUST
			dhcpv4.WithOption(dhcpv4.OptServerIdentifier(serverIP)),
		)
		if err != nil {
			log.F("[dpcpd] %s: can not create reply message, error: %s", d.name, err)
			return
		}

		if err := reply(d.iface, resp); err != nil {
			log.F("[dpcpd] %s: could not write to %v(%v): %s",
				d.name, resp.ClientHWAddr, peer, err)
			return
		}

		log.F("[dpcpd] %s: %s to %v for %v",
			d.name, replyType, resp.ClientHWAddr, replyIP)

	}
}

func (d *dhcpd) isFailover() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.failover
}

func (d *dhcpd) setFailover(v bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.failover != v {
		if v {
			log.F("[dpcpd] %s: dhcp server detected, enter failover mode", d.iface.Name)
		} else {
			log.F("[dpcpd] %s: no dhcp server detected, exit failover mode and serve requests", d.iface.Name)
		}
	}
	d.failover = v
}

func ifaceAddr(iface string) (*net.Interface, net.IP, net.IPMask, error) {
	intf, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, nil, nil, err
	}

	addrs, err := intf.Addrs()
	if err != nil {
		return intf, nil, nil, err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.IsLoopback() {
				return intf, nil, nil, errors.New("can't use loopback interface")
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return intf, ip4, ipnet.Mask, nil
			}
		}
	}

	return intf, nil, nil, errors.New("no ip/mask defined on this interface")
}
