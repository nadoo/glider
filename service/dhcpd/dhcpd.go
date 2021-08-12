package dhcpd

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/service"
)

var leaseTime = time.Hour * 12

func init() {
	service.Register("dhcpd", &dpcpd{})
}

type dpcpd struct{}

// Run runs the service.
func (*dpcpd) Run(args ...string) {
	if len(args) < 4 {
		log.F("[dhcpd] not enough parameters, exiting")
		return
	}

	iface, startIP, endIP, leaseMin := args[0], args[1], args[2], args[3]
	if i, err := strconv.Atoi(leaseMin); err != nil {
		leaseTime = time.Duration(i) * time.Minute
	}

	ip, mask, _, err := ifaceAddr(iface)
	if err != nil {
		log.F("[dhcpd] get ip of interface '%s' error: %s", iface, err)
		return
	}

	if existsServer(iface) {
		log.F("[dhcpd] found existing dhcp server on interface %s, service exiting", iface)
		return
	}

	pool, err := NewPool(leaseTime, net.ParseIP(startIP), net.ParseIP(endIP))
	if err != nil {
		log.F("[dhcpd] error in pool init: %s", err)
		return
	}

	// static ips
	for _, host := range args[4:] {
		pair := strings.Split(host, "=")
		if len(pair) == 2 {
			mac, err := net.ParseMAC(pair[0])
			if err != nil {
				break
			}
			ip := net.ParseIP(pair[1])
			pool.LeaseStaticIP(mac, ip)
		}
	}

	laddr := net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: 67}
	server, err := server4.NewServer(iface, &laddr, handleDHCP(ip, mask, pool))
	if err != nil {
		log.F("[dhcpd] error in server creation: %s", err)
		return
	}

	log.F("[dhcpd] Listening on interface %s(%s/%d.%d.%d.%d)",
		iface, ip, mask[0], mask[1], mask[2], mask[3])

	server.Serve()
}

func handleDHCP(serverIP net.IP, mask net.IPMask, pool *Pool) server4.Handler {
	return func(conn net.PacketConn, peer net.Addr, m *dhcpv4.DHCPv4) {

		var replyType dhcpv4.MessageType
		switch mt := m.MessageType(); mt {
		case dhcpv4.MessageTypeDiscover:
			replyType = dhcpv4.MessageTypeOffer
		case dhcpv4.MessageTypeRequest, dhcpv4.MessageTypeInform:
			replyType = dhcpv4.MessageTypeAck
		case dhcpv4.MessageTypeRelease:
			pool.ReleaseIP(m.ClientHWAddr)
			log.F("[dpcpd] %v released ip %v", m.ClientHWAddr, m.ClientIPAddr)
			return
		case dhcpv4.MessageTypeDecline:
			pool.ReleaseIP(m.ClientHWAddr)
			log.F("[dpcpd] received decline message from %v", m.ClientHWAddr)
			return
		default:
			log.F("[dpcpd] can't handle type %v", mt)
			return
		}

		replyIp, err := pool.LeaseIP(m.ClientHWAddr)
		if err != nil {
			log.F("[dpcpd] can not assign IP, error %s", err)
			return
		}

		reply, err := dhcpv4.NewReplyFromRequest(m,
			dhcpv4.WithMessageType(replyType),
			dhcpv4.WithServerIP(serverIP),
			dhcpv4.WithNetmask(mask),
			dhcpv4.WithYourIP(replyIp),
			dhcpv4.WithRouter(serverIP),
			dhcpv4.WithDNS(serverIP),
			// RFC 2131, Section 4.3.1. Server Identifier: MUST
			dhcpv4.WithOption(dhcpv4.OptServerIdentifier(serverIP)),
			// RFC 2131, Section 4.3.1. IP lease time: MUST
			dhcpv4.WithOption(dhcpv4.OptIPAddressLeaseTime(leaseTime)),
		)
		if err != nil {
			log.F("[dpcpd] can not create reply message, error %s", err)
			return
		}

		if val := m.Options.Get(dhcpv4.OptionClientIdentifier); len(val) > 0 {
			reply.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionClientIdentifier, val))
		}

		if _, err := conn.WriteTo(reply.ToBytes(), peer); err != nil {
			log.F("[dpcpd] could not write to client %s(%s): %s", peer, reply.ClientHWAddr, err)
			return
		}

		log.F("[dpcpd] lease %v to client %v", replyIp, reply.ClientHWAddr)
	}
}

func ifaceAddr(iface string) (net.IP, net.IPMask, net.HardwareAddr, error) {
	intf, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, nil, nil, err
	}

	addrs, err := intf.Addrs()
	if err != nil {
		return nil, nil, intf.HardwareAddr, err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.IsLoopback() {
				return nil, nil, intf.HardwareAddr, errors.New("can't use loopback interface")
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4, ipnet.Mask, intf.HardwareAddr, nil
			}
		}
	}

	return nil, nil, intf.HardwareAddr, errors.New("no ip/mask defined on this interface")
}
