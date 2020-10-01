package dhcpd

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/service"
)

var leaseTime = time.Hour * 12

func init() {
	service.Register("dhcpd", &dpcpd{})
}

type dpcpd struct{}

// Run runs the service.
func (*dpcpd) Run(args ...string) {
	if len(args) < 3 {
		log.F("[dhcpd] not enough parameters, exiting")
		return
	}

	iface := args[0]
	ip, mask, err := intfaceIP4(iface)
	if err != nil {
		log.F("[dhcpd] get ip of interface '%s' error: %s", iface, err)
		return
	}

	if findExistServer(iface) {
		log.F("[dhcpd] found existing dhcp server on interface %s, service exiting", iface)
		return
	}

	pool, err := NewPool(leaseTime, net.ParseIP(args[1]), net.ParseIP(args[2]))
	if err != nil {
		log.F("[dhcpd] error in pool init: %s", err)
		return
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
		case dhcpv4.MessageTypeRequest:
			replyType = dhcpv4.MessageTypeAck
		default:
			log.F("[dpcpd] can't handle type %v", mt)
			return
		}

		replyIp, err := pool.AssignIP(m.ClientHWAddr)
		if err != nil {
			log.F("[dpcpd] can not assign IP error %s", err)
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

func findExistServer(iface string) (exists bool) {
	client, err := nclient4.New(iface)
	if err != nil {
		log.F("[dhcpd] failed in dhcp client creation: %s", err)
		return
	}
	defer client.Close()

	ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = client.Request(ctx)
	if err != nil {
		return
	}

	return true
}

func intfaceIP4(iface string) (net.IP, net.IPMask, error) {
	intf, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, nil, err
	}

	addrs, err := intf.Addrs()
	if err != nil {
		return nil, nil, err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.IsLoopback() {
				return nil, nil, errors.New("can't use loopback interface")
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4, ipnet.Mask, nil
			}
		}
	}

	return nil, nil, errors.New("no ip/mask defined on this interface")
}
