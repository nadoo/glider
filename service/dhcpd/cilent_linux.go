package dhcpd

import (
	"context"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"

	"github.com/nadoo/glider/log"
)

func existsServer(iface string) (exists bool) {
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
