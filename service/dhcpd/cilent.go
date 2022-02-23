package dhcpd

import (
	"context"
	"net"
	"runtime"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/sockopt"
)

func existsServer(intf *net.Interface) (exists bool) {
	lc := &net.ListenConfig{}
	lc.Control = sockopt.BindControl(intf)

	addr := "0.0.0.0:68"
	if runtime.GOOS == "linux" || runtime.GOOS == "android" {
		addr = "255.255.255.255:68"
	}

	pc, err := lc.ListenPacket(context.Background(), "udp4", addr)
	if err != nil {
		log.F("[dhcpd] failed in dhcp client ListenPacket: %s", err)
		return
	}
	defer pc.Close()

	discovery, err := dhcpv4.NewDiscovery(intf.HardwareAddr, dhcpv4.WithBroadcast(true))
	if err != nil {
		log.F("[dhcpd] failed in dhcp client NewDiscovery: %s", err)
		return
	}

	_, err = pc.WriteTo(discovery.ToBytes(), &net.UDPAddr{IP: net.IPv4bcast, Port: 67})
	if err != nil {
		log.F("[dhcpd] failed in dhcp client WriteTo: %s", err)
		return
	}

	var buf [dhcpv4.MaxMessageSize]byte
	pc.SetReadDeadline(time.Now().Add(time.Second * 3))
	n, _, err := pc.ReadFrom(buf[:])
	if err != nil {
		return
	}

	msg, err := dhcpv4.FromBytes(buf[:n])
	if err != nil || msg.TransactionID != discovery.TransactionID {
		return
	}

	return true
}
