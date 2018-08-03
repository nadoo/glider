package main

import (
	stdlog "log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/dns"
	"github.com/nadoo/glider/proxy"

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
const VERSION = "0.6.6"

func dialerFromConf() proxy.Dialer {
	// global forwarders in xx.conf
	var fwdrs []proxy.Dialer
	for _, chain := range conf.Forward {
		var fwdr proxy.Dialer
		var err error
		for _, url := range strings.Split(chain, ",") {
			fwdr, err = proxy.DialerFromURL(url, fwdr)
			if err != nil {
				log.Fatal(err)
			}
		}
		fwdrs = append(fwdrs, fwdr)
	}

	return NewStrategyDialer(conf.Strategy, fwdrs, conf.CheckWebSite, conf.CheckDuration)
}

func main() {

	confInit()
	log.F = func(f string, v ...interface{}) {
		if conf.Verbose {
			stdlog.Printf(f, v...)
		}
	}

	dialer := NewRuleDialer(conf.rules, dialerFromConf())
	ipsetM, _ := NewIPSetManager(conf.IPSet, conf.rules)
	if conf.DNS != "" {
		d, err := dns.NewServer(conf.DNS, dialer, conf.DNSServer)
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

	for _, listen := range conf.Listen {
		local, err := proxy.ServerFromURL(listen, dialer)
		if err != nil {
			log.Fatal(err)
		}

		go local.ListenAndServe()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
