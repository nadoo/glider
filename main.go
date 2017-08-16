package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nadoo/conflag"
)

// VERSION .
const VERSION = "0.3.2"

var conf struct {
	Verbose       bool
	Strategy      string
	CheckWebSite  string
	CheckDuration int
	Listen        []string
	Forward       []string
	RuleFile      []string

	DNS       string
	DNSServer []string

	IPSet string
}

var flag = conflag.New()

func logf(f string, v ...interface{}) {
	if conf.Verbose {
		log.Printf(f, v...)
	}
}

func usage() {
	app := os.Args[0]
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "%s v%s usage:\n", app, VERSION)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Available Schemas:\n")
	fmt.Fprintf(os.Stderr, "  mixed: serve as a http/socks5 proxy on the same port. (default)\n")
	fmt.Fprintf(os.Stderr, "  ss: ss proxy\n")
	fmt.Fprintf(os.Stderr, "  socks5: socks5 proxy\n")
	fmt.Fprintf(os.Stderr, "  http: http proxy\n")
	fmt.Fprintf(os.Stderr, "  redir: redirect proxy. (used on linux as a transparent proxy with iptables redirect rules)\n")
	fmt.Fprintf(os.Stderr, "  tcptun: a simple tcp tunnel\n")
	fmt.Fprintf(os.Stderr, "  dnstun: listen on udp port and forward all dns requests to remote dns server via forwarders(tcp)\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Available schemas for different modes:\n")
	fmt.Fprintf(os.Stderr, "  listen: mixed ss socks5 http redir tcptun dnstun\n")
	fmt.Fprintf(os.Stderr, "  forward: ss socks5 http\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Available methods for ss:\n")
	fmt.Fprintf(os.Stderr, "  "+ListCipher())
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  NOTE: chacha20-ietf-poly1305 = AEAD_CHACHA20_POLY1305\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Available forward strategies:\n")
	fmt.Fprintf(os.Stderr, "  rr: Round Robin mode\n")
	fmt.Fprintf(os.Stderr, "  ha: High Availability mode\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Config file format(see `"+app+".conf.example` as an example):\n")
	fmt.Fprintf(os.Stderr, "  # COMMENT LINE\n")
	fmt.Fprintf(os.Stderr, "  KEY=VALUE\n")
	fmt.Fprintf(os.Stderr, "  KEY=VALUE\n")
	fmt.Fprintf(os.Stderr, "  # KEY equals to command line flag name: listen forward strategy...\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Examples:\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -config glider.conf\n")
	fmt.Fprintf(os.Stderr, "    -run glider with specified config file.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -config glider.conf -rulefile office.rule -rulefile home.rule\n")
	fmt.Fprintf(os.Stderr, "    -run glider with specified global config file and rule config files.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen :8443\n")
	fmt.Fprintf(os.Stderr, "    -listen on :8443, serve as http/socks5 proxy on the same port.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen ss://AEAD_CHACHA20_POLY1305:pass@:8443\n")
	fmt.Fprintf(os.Stderr, "    -listen on 0.0.0.0:8443 as a ss server.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen socks5://:1080 -verbose\n")
	fmt.Fprintf(os.Stderr, "    -listen on :1080 as a socks5 proxy server, in verbose mode.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen http://:8080 -forward socks5://127.0.0.1:1080\n")
	fmt.Fprintf(os.Stderr, "    -listen on :8080 as a http proxy server, forward all requests via socks5 server.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen redir://:1081 -forward ss://method:pass@1.1.1.1:8443\n")
	fmt.Fprintf(os.Stderr, "    -listen on :1081 as a transparent redirect server, forward all requests via remote ss server.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen tcptun://:80=2.2.2.2:80 -forward ss://method:pass@1.1.1.1:8443\n")
	fmt.Fprintf(os.Stderr, "    -listen on :80 and forward all requests to 2.2.2.2:80 via remote ss server.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen socks5://:1080 -listen http://:8080 -forward ss://method:pass@1.1.1.1:8443\n")
	fmt.Fprintf(os.Stderr, "    -listen on :1080 as socks5 server, :8080 as http proxy server, forward all requests via remote ss server.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen redir://:1081 -listen dnstun://:53=8.8.8.8:53 -forward ss://method:pass@server1:port1,ss://method:pass@server2:port2\n")
	fmt.Fprintf(os.Stderr, "    -listen on :1081 as transparent redirect server, :53 as dns server, use forward chain: server1 -> server2.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen socks5://:1080 -forward ss://method:pass@server1:port1 -forward ss://method:pass@server2:port2 -strategy rr\n")
	fmt.Fprintf(os.Stderr, "    -listen on :1080 as socks5 server, forward requests via server1 and server2 in roundrbin mode.\n")
	fmt.Fprintf(os.Stderr, "\n")
}

func main() {

	flag.BoolVar(&conf.Verbose, "verbose", false, "verbose mode")
	flag.StringVar(&conf.Strategy, "strategy", "rr", "forward strategy, default: rr")
	flag.StringVar(&conf.CheckWebSite, "checkwebsite", "www.apple.com", "proxy check HTTP(NOT HTTPS) website address, format: HOST[:PORT], default port: 80")
	flag.IntVar(&conf.CheckDuration, "checkduration", 30, "proxy check duration(seconds)")
	flag.StringSliceUniqVar(&conf.Listen, "listen", nil, "listen url, format: SCHEMA://[USER|METHOD:PASSWORD@][HOST]:PORT")
	flag.StringSliceUniqVar(&conf.Forward, "forward", nil, "forward url, format: SCHEMA://[USER|METHOD:PASSWORD@][HOST]:PORT[,SCHEMA://[USER|METHOD:PASSWORD@][HOST]:PORT]")
	flag.StringSliceUniqVar(&conf.RuleFile, "rulefile", nil, "rule file path")

	flag.StringVar(&conf.DNS, "dns", "", "dns listen address")
	flag.StringSliceUniqVar(&conf.DNSServer, "dnsserver", []string{"8.8.8.8:53"}, "remote dns server")

	flag.StringVar(&conf.IPSet, "ipset", "glider", "ipset name")

	flag.Usage = usage
	err := flag.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		return
	}

	if len(conf.Listen) == 0 && conf.DNS == "" {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "ERROR: listen url must be specified.\n")
		return
	}

	// global forwarders in xx.conf
	var forwarders []Proxy
	for _, chain := range conf.Forward {
		var forward Proxy
		var err error
		for _, url := range strings.Split(chain, ",") {
			forward, err = ProxyFromURL(url, forward)
			if err != nil {
				log.Fatal(err)
			}
		}
		forwarders = append(forwarders, forward)
	}

	// combine forwarders to a singer strategy forwarder
	forwarder := newStrategyForwarder(conf.Strategy, forwarders)

	// rule forwarders
	var ruleForwarders []*ruleForwarder
	for _, ruleFile := range conf.RuleFile {
		ruleForwarder, err := newRuleProxyFromFile(ruleFile)
		if err != nil {
			log.Fatal(err)
		}

		ruleForwarders = append(ruleForwarders, ruleForwarder)
	}

	// combine ruleforwarders and global strategy forwarder
	forwarder = newRulesForwarder(ruleForwarders, forwarder)

	for _, listen := range conf.Listen {
		local, err := ProxyFromURL(listen, forwarder)
		if err != nil {
			log.Fatal(err)
		}

		go local.ListenAndServe()
	}

	if len(forwarders) > 1 {
		for _, forward := range forwarders {
			go check(forward, conf.CheckWebSite, conf.CheckDuration)
		}
	}

	if conf.DNS != "" {
		dns, err := DNSForwarder(conf.DNS, conf.DNSServer[0], forwarder)
		if err != nil {
			log.Fatal(err)
		}

		// rule
		for _, frwder := range ruleForwarders {
			for _, domain := range frwder.Domain {
				if len(frwder.DNSServer) > 0 {
					dns.SetServer(domain, frwder.DNSServer[0])
				}
			}
		}

		go dns.ListenAndServe()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
