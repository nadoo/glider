package dhcpd

import (
	"context"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"

	"github.com/nadoo/glider/pkg/sockopt"
)

func discovery(intf *net.Interface) (found bool) {
	lc := &net.ListenConfig{Control: sockopt.Control(sockopt.Bind(intf), sockopt.ReuseAddr())}

	pc, err := lc.ListenPacket(context.Background(), "udp4", ":68")
	if err != nil {
		return
	}
	defer pc.Close()

	discovery, err := dhcpv4.NewDiscovery(intf.HardwareAddr, dhcpv4.WithBroadcast(true))
	if err != nil {
		return
	}

	_, err = pc.WriteTo(discovery.ToBytes(), &net.UDPAddr{IP: net.IPv4bcast, Port: 67})
	if err != nil {
		return
	}

	var buf [dhcpv4.MaxMessageSize]byte
	pc.SetReadDeadline(time.Now().Add(time.Second * 3))
	n, _, err := pc.ReadFrom(buf[:])
	if err != nil {
		return
	}

	_, err = dhcpv4.FromBytes(buf[:n])
	return err == nil
}
