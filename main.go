package main

import (
	stdlog "log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/dns"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/strategy"

	_ "github.com/nadoo/glider/proxy/http"
	_ "github.com/nadoo/glider/proxy/mixed"
	_ "github.com/nadoo/glider/proxy/socks5"
	_ "github.com/nadoo/glider/proxy/ss"
	_ "github.com/nadoo/glider/proxy/ssr"
	_ "github.com/nadoo/glider/proxy/tcptun"
	_ "github.com/nadoo/glider/proxy/tls"
	_ "github.com/nadoo/glider/proxy/udptun"
	_ "github.com/nadoo/glider/proxy/uottun"
	_ "github.com/nadoo/glider/proxy/vmess"
	_ "github.com/nadoo/glider/proxy/ws"
)

// VERSION .
const VERSION = "0.6.7"

func main() {
	confInit()
	log.F = func(f string, v ...interface{}) {
		if conf.Verbose {
			stdlog.Printf(f, v...)
		}
	}

	dialer := NewRuleDialer(conf.rules, strategy.NewDialer(conf.Forward, &conf.StrategyConfig))
	ipsetM, _ := NewIPSetManager(conf.IPSet, conf.rules)

	// DNS Server
	if conf.DNS != "" {
		dnscfg := &dns.Config{
			Timeout: conf.DNSTimeout,
			MaxTTL:  conf.DNSMaxTTL,
			MinTTL:  conf.DNSMinTTL}

		d, err := dns.NewServer(conf.DNS, dialer, conf.DNSServer, dnscfg)
		if err != nil {
			log.Fatal(err)
		}

		// custom records
		for _, record := range conf.DNSRecord {
			d.AddRecord(record)
		}

		// rule
		for _, r := range conf.rules {
			for _, domain := range r.Domain {
				if len(r.DNSServer) > 0 {
					d.SetServer(domain, r.DNSServer...)
				}
			}
		}

		// add a handler to update proxy rules when a domain resolved
		d.AddHandler(dialer.AddDomainIP)
		if ipsetM != nil {
			d.AddHandler(ipsetM.AddDomainIP)
		}

		go d.ListenAndServe()
	}

	// Proxy Servers
	for _, listen := range conf.Listen {
		local, err := proxy.ServerFromURL(listen, proxy.NewForwarder(dialer))
		if err != nil {
			log.Fatal(err)
		}

		go local.ListenAndServe()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
