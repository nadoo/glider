package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// VERSION .
const VERSION = "0.4.3"

func dialerFromConf() Dialer {
	// global forwarders in xx.conf
	var fwdrs []Dialer
	for _, chain := range conf.Forward {
		var fwdr Dialer
		var err error
		for _, url := range strings.Split(chain, ",") {
			fwdr, err = DialerFromURL(url, fwdr)
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
	sDialer := NewRuleDialer(conf.rules, dialerFromConf())

	for _, listen := range conf.Listen {
		local, err := ServerFromURL(listen, sDialer)
		if err != nil {
			log.Fatal(err)
		}

		go local.ListenAndServe()
	}

	ipsetM, err := NewIPSetManager(conf.IPSet, conf.rules)
	if err != nil {
		logf("create ipset manager error: %s", err)
	}

	if conf.DNS != "" {
		dns, err := NewDNS(conf.DNS, conf.DNSServer[0], sDialer, false)
		if err != nil {
			log.Fatal(err)
		}

		// rule
		for _, r := range conf.rules {
			for _, domain := range r.Domain {
				if len(r.DNSServer) > 0 {
					dns.SetServer(domain, r.DNSServer[0])
				}
			}
		}

		// add a handler to update proxy rules when a domain resolved
		dns.AddAnswerHandler(sDialer.AddDomainIP)
		if ipsetM != nil {
			dns.AddAnswerHandler(ipsetM.AddDomainIP)
		}

		go dns.ListenAndServe()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
