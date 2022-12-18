package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nadoo/glider/dns"
	"github.com/nadoo/glider/ipset"
	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/rule"
	"github.com/nadoo/glider/service"
)

var (
	version = "0.16.3"
	config  = parseConfig()
)

func main() {
	// global rule proxy
	pxy := rule.NewProxy(config.Forwards, &config.Strategy, config.rules)

	// ipset manager
	ipsetM, _ := ipset.NewManager(config.rules)

	// check and setup dns server
	if config.DNS != "" {
		d, err := dns.NewServer(config.DNS, pxy, &config.DNSConfig)
		if err != nil {
			log.Fatal(err)
		}

		// rules
		for _, r := range config.rules {
			if len(r.DNSServers) > 0 {
				for _, domain := range r.Domain {
					d.SetServers(domain, r.DNSServers)
				}
			}
		}

		// add a handler to update proxy rules when a domain resolved
		d.AddHandler(pxy.AddDomainIP)
		if ipsetM != nil {
			d.AddHandler(ipsetM.AddDomainIP)
		}

		d.Start()

		// custom resolver
		net.DefaultResolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: time.Second * 3}
				return d.DialContext(ctx, "udp", config.DNS)
			},
		}
	}

	for _, r := range config.rules {
		r.IP, r.CIDR, r.Domain = nil, nil, nil
	}

	// enable checkers
	pxy.Check()

	// run proxy servers
	for _, listen := range config.Listens {
		local, err := proxy.ServerFromURL(listen, pxy)
		if err != nil {
			log.Fatal(err)
		}
		go local.ListenAndServe()
	}

	// run services
	for _, s := range config.Services {
		service, err := service.New(s)
		if err != nil {
			log.Fatal(err)
		}
		go service.Run()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
