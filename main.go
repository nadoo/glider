package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// VERSION .
const VERSION = "0.4.0"

func logf(f string, v ...interface{}) {
	if conf.Verbose {
		log.Printf(f, v...)
	}
}

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

	forwarder := NewStrategyDialer(conf.Strategy, forwarders, conf.CheckWebSite, conf.CheckDuration)

	return NewRuleDialer(conf.rules, forwarder)
}

func main() {

	confInit()
	sDialer := dialerFromConf()

	for _, listen := range conf.Listen {
		local, err := ServerFromURL(listen, sDialer)
		if err != nil {
			log.Fatal(err)
		}

		go local.ListenAndServe()
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

		// test here
		dns.AddAnswerHandler(func(domain, ip string) error {
			if ip != "" {
				logf("domain: %s, ip: %s\n", domain, ip)
			}
			return nil
		})

		go dns.ListenAndServe()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
