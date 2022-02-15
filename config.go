package main

import (
	"fmt"
	"os"
	"path"

	"github.com/nadoo/conflag"

	"github.com/nadoo/glider/dns"
	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/rule"
)

var flag = conflag.New()

// Config is global config struct.
type Config struct {
	Verbose    bool
	LogFlags   int
	TCPBufSize int
	UDPBufSize int

	Listens []string

	Forwards []string
	Strategy rule.Strategy

	RuleFiles []string
	RulesDir  string

	DNS       string
	DNSConfig dns.Config

	rules []*rule.Config

	Services []string
}

func parseConfig() *Config {
	conf := &Config{}

	flag.SetOutput(os.Stdout)

	scheme := flag.String("scheme", "", "show help message of proxy scheme, use 'all' to see all ")

	flag.BoolVar(&conf.Verbose, "verbose", false, "verbose mode")
	flag.IntVar(&conf.LogFlags, "logflags", 19, "do not change it if you do not know what it is, ref: https://pkg.go.dev/log#pkg-constants")
	flag.IntVar(&conf.TCPBufSize, "tcpbufsize", 32768, "tcp buffer size in Bytes")
	flag.IntVar(&conf.UDPBufSize, "udpbufsize", 2048, "udp buffer size in Bytes")
	flag.StringSliceUniqVar(&conf.Listens, "listen", nil, "listen url, see the URL section below")

	flag.StringSliceVar(&conf.Forwards, "forward", nil, "forward url, see the URL section below")
	flag.StringVar(&conf.Strategy.Strategy, "strategy", "rr", `rr: Round Robin mode
ha: High Availability mode
lha: Latency based High Availability mode
dh: Destination Hashing mode`)
	flag.StringVar(&conf.Strategy.Check, "check", "http://www.msftconnecttest.com/connecttest.txt#expect=200",
		`check=tcp[://HOST:PORT]: tcp port connect check
check=http://HOST[:PORT][/URI][#expect=REGEX_MATCH_IN_RESP_LINE]
check=https://HOST[:PORT][/URI][#expect=REGEX_MATCH_IN_RESP_LINE]
check=file://SCRIPT_PATH: run a check script, healthy when exitcode=0, env vars: FORWARDER_ADDR,FORWARDER_URL
check=disable: disable health check`)
	flag.IntVar(&conf.Strategy.CheckInterval, "checkinterval", 30, "fowarder check interval(seconds)")
	flag.IntVar(&conf.Strategy.CheckTimeout, "checktimeout", 10, "fowarder check timeout(seconds)")
	flag.IntVar(&conf.Strategy.CheckTolerance, "checktolerance", 0, "fowarder check tolerance(ms), switch only when new_latency < old_latency - tolerance, only used in lha mode")
	flag.BoolVar(&conf.Strategy.CheckDisabledOnly, "checkdisabledonly", false, "check disabled fowarders only")
	flag.IntVar(&conf.Strategy.MaxFailures, "maxfailures", 3, "max failures to change forwarder status to disabled")
	flag.IntVar(&conf.Strategy.DialTimeout, "dialtimeout", 3, "dial timeout(seconds)")
	flag.IntVar(&conf.Strategy.RelayTimeout, "relaytimeout", 0, "relay timeout(seconds)")
	flag.StringVar(&conf.Strategy.IntFace, "interface", "", "source ip or source interface")

	flag.StringSliceUniqVar(&conf.RuleFiles, "rulefile", nil, "rule file path")
	flag.StringVar(&conf.RulesDir, "rules-dir", "", "rule file folder")

	// dns configs
	flag.StringVar(&conf.DNS, "dns", "", "local dns server listen address")
	flag.StringSliceUniqVar(&conf.DNSConfig.Servers, "dnsserver", []string{"8.8.8.8:53"}, "remote dns server address")
	flag.BoolVar(&conf.DNSConfig.AlwaysTCP, "dnsalwaystcp", false, "always use tcp to query upstream dns servers no matter there is a forwarder or not")
	flag.IntVar(&conf.DNSConfig.Timeout, "dnstimeout", 3, "timeout value used in multiple dnsservers switch(seconds)")
	flag.IntVar(&conf.DNSConfig.MaxTTL, "dnsmaxttl", 1800, "maximum TTL value for entries in the CACHE(seconds)")
	flag.IntVar(&conf.DNSConfig.MinTTL, "dnsminttl", 0, "minimum TTL value for entries in the CACHE(seconds)")
	flag.IntVar(&conf.DNSConfig.CacheSize, "dnscachesize", 4096, "max number of dns response in CACHE")
	flag.BoolVar(&conf.DNSConfig.CacheLog, "dnscachelog", false, "show query log of dns cache")
	flag.BoolVar(&conf.DNSConfig.NoAAAA, "dnsnoaaaa", false, "disable AAAA query")
	flag.StringSliceUniqVar(&conf.DNSConfig.Records, "dnsrecord", nil, "custom dns record, format: domain/ip")

	// service configs
	flag.StringSliceUniqVar(&conf.Services, "service", nil, "run specified services, format: SERVICE_NAME[,SERVICE_CONFIG]")

	flag.Usage = usage
	err := flag.Parse()
	if err != nil {
		// flag.Usage()
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(-1)
	}

	if *scheme != "" {
		fmt.Fprintf(os.Stdout, proxy.Usage(*scheme))
		os.Exit(0)
	}

	// setup logger
	log.Set(conf.Verbose, conf.LogFlags)

	if len(conf.Listens) == 0 && conf.DNS == "" && len(conf.Services) == 0 {
		// flag.Usage()
		fmt.Fprintf(os.Stderr, "ERROR: listen url must be specified.\n")
		os.Exit(-1)
	}

	// tcpbufsize
	if conf.TCPBufSize > 0 {
		proxy.TCPBufSize = conf.TCPBufSize
	}

	// udpbufsize
	if conf.UDPBufSize > 0 {
		proxy.UDPBufSize = conf.UDPBufSize
	}

	// rulefiles
	for _, ruleFile := range conf.RuleFiles {
		if !path.IsAbs(ruleFile) {
			ruleFile = path.Join(flag.ConfDir(), ruleFile)
		}

		rule, err := rule.NewConfFromFile(ruleFile)
		if err != nil {
			log.Fatal(err)
		}

		conf.rules = append(conf.rules, rule)
	}

	if conf.RulesDir != "" {
		if !path.IsAbs(conf.RulesDir) {
			conf.RulesDir = path.Join(flag.ConfDir(), conf.RulesDir)
		}

		ruleFolderFiles, _ := rule.ListDir(conf.RulesDir, ".rule")
		for _, ruleFile := range ruleFolderFiles {
			rule, err := rule.NewConfFromFile(ruleFile)
			if err != nil {
				log.Fatal(err)
			}
			conf.rules = append(conf.rules, rule)
		}
	}

	return conf
}

func usage() {
	w := flag.Output()

	fmt.Fprint(w, `
Usage: glider [-listen URL]... [-forward URL]... [OPTION]...
  e.g. glider -config /etc/glider/glider.conf
       glider -listen :8443 -forward socks5://serverA:1080 -forward socks5://serverB:1080 -verbose

`)

	fmt.Fprintf(w, "OPTION:\n")
	flag.PrintDefaults()

	fmt.Fprint(w, `
URL:
   proxy: SCHEME://[USER:PASS@][HOST]:PORT
   chain: proxy,proxy,[proxy]...

    e.g. -listen socks5://:1080
         -listen tls://:443?cert=crtFilePath&key=keyFilePath,http://    (protocol chain)

    e.g. -forward socks5://server:1080
         -forward tls://server.com:443,http://                          (protocol chain)
         -forward socks5://serverA:1080,socks5://serverB:1080           (proxy chain)

`)

	fmt.Fprintf(w, "SCHEME:\n")
	fmt.Fprintf(w, "   listen : %s\n", proxy.ServerSchemes())
	fmt.Fprintf(w, "   forward: %s\n", proxy.DialerSchemes())
	fmt.Fprintf(w, "\n   Note: use `glider -scheme all` or `glider -scheme SCHEME` to see help info for the scheme.\n")

	fmt.Fprint(w, `
--
Forwarder Options: FORWARD_URL#OPTIONS
   priority : the priority of that forwarder, the larger the higher, default: 0
   interface: the local interface or ip address used to connect remote server.

   e.g. -forward socks5://server:1080#priority=100
        -forward socks5://server:1080#interface=eth0
        -forward socks5://server:1080#priority=100&interface=192.168.1.99

Services:
   dhcpd: service=dhcpd,INTERFACE,START_IP,END_IP,LEASE_MINUTES[,MAC=IP,MAC=IP...]
     e.g. service=dhcpd,eth1,192.168.1.100,192.168.1.199,720

see README.md and glider.conf.example for more details.
`)

	fmt.Fprintf(w, "--\nglider v%s, https://github.com/nadoo/glider\n\n", version)
}
