package main

import (
	"fmt"
	stdlog "log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dongxinb/glider/common/log"
	"github.com/dongxinb/glider/dns"
	"github.com/dongxinb/glider/ipset"
	"github.com/dongxinb/glider/proxy"
	"github.com/dongxinb/glider/rule"
	"github.com/dongxinb/glider/strategy"

	_ "github.com/dongxinb/glider/proxy/http"
	_ "github.com/dongxinb/glider/proxy/kcp"
	_ "github.com/dongxinb/glider/proxy/mixed"
	_ "github.com/dongxinb/glider/proxy/obfs"
	_ "github.com/dongxinb/glider/proxy/reject"
	_ "github.com/dongxinb/glider/proxy/socks5"
	_ "github.com/dongxinb/glider/proxy/ss"
	_ "github.com/dongxinb/glider/proxy/ssr"
	_ "github.com/dongxinb/glider/proxy/tcptun"
	_ "github.com/dongxinb/glider/proxy/tls"
	_ "github.com/dongxinb/glider/proxy/udptun"
	_ "github.com/dongxinb/glider/proxy/uottun"
	_ "github.com/dongxinb/glider/proxy/vmess"
	_ "github.com/dongxinb/glider/proxy/ws"
)

var version = "0.9.3"

func main() {
	// read configs
	confInit()

	// setup a log func
	log.F = func(f string, v ...interface{}) {
		if conf.Verbose {
			stdlog.Output(2, fmt.Sprintf(f, v...))
		}
	}

	// global rule proxy
	p := rule.NewProxy(conf.rules, strategy.NewProxy(conf.Forward, &conf.StrategyConfig))

	// ipset manager
	ipsetM, _ := ipset.NewManager(conf.rules)

	// check and setup dns server
	if conf.DNS != "" {
		d, err := dns.NewServer(conf.DNS, p, &conf.DNSConfig)
		if err != nil {
			log.Fatal(err)
		}

		// rule
		for _, r := range conf.rules {
			for _, domain := range r.Domain {
				if len(r.DNSServers) > 0 {
					d.SetServers(domain, r.DNSServers...)
				}
			}
		}

		// add a handler to update proxy rules when a domain resolved
		d.AddHandler(p.AddDomainIP)
		if ipsetM != nil {
			d.AddHandler(ipsetM.AddDomainIP)
		}

		d.Start()
	}

	// enable checkers
	p.Check()

	// Proxy Servers
	for _, listen := range conf.Listen {
		local, err := proxy.ServerFromURL(listen, p)
		if err != nil {
			log.Fatal(err)
		}

		go local.ListenAndServe()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
