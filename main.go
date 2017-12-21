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
	var forwarders []Dialer
	for _, chain := range conf.Forward {
		var forward Dialer
		var err error
		for _, url := range strings.Split(chain, ",") {
			forward, err = DialerFromURL(url, forward)
			if err != nil {
				log.Fatal(err)
			}
		}
		forwarders = append(forwarders, forward)
	}

	return NewStrategyDialer(conf.Strategy, forwarders, conf.CheckWebSite, conf.CheckDuration)
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
		dns, err := NewDNS(conf.DNS, conf.DNSServer[0], sDialer)
		if err != nil {
			log.Fatal(err)
		}

		// rule
		for _, frwder := range conf.rules {
			for _, domain := range frwder.Domain {
				if len(frwder.DNSServer) > 0 {
					dns.SetServer(domain, frwder.DNSServer[0])
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
