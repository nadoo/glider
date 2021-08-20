package main

import (
	"fmt"
	"os"
	"path"

	"github.com/nadoo/conflag"

	"github.com/nadoo/glider/dns"
	"github.com/nadoo/glider/log"
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

	flag.BoolVar(&conf.Verbose, "verbose", false, "verbose mode")
	flag.IntVar(&conf.LogFlags, "logflags", 19, "log flags, do not change it if you do not know what it is, ref: https://pkg.go.dev/log#pkg-constants")
	flag.IntVar(&conf.TCPBufSize, "tcpbufsize", 32768, "tcp buffer size in Bytes")
	flag.IntVar(&conf.UDPBufSize, "udpbufsize", 2048, "udp buffer size in Bytes")
	flag.StringSliceUniqVar(&conf.Listens, "listen", nil, "listen url, format: SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS")

	flag.StringSliceUniqVar(&conf.Forwards, "forward", nil, "forward url, format: SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS[,SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS]")
	flag.StringVar(&conf.Strategy.Strategy, "strategy", "rr", "forward strategy, default: rr")
	flag.StringVar(&conf.Strategy.Check, "check", "http://www.msftconnecttest.com/connecttest.txt#expect=200", "check=tcp[://HOST:PORT]: tcp port connect check\ncheck=http://HOST[:PORT][/URI][#expect=STRING_IN_RESP_LINE]\ncheck=file://SCRIPT_PATH: run a check script, healthy when exitcode=0, environment variables: FORWARDER_ADDR\ncheck=disable: disable health check")
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
	flag.IntVar(&conf.DNSConfig.CacheSize, "dnscachesize", 4096, "size of CACHE")
	flag.BoolVar(&conf.DNSConfig.CacheLog, "dnscachelog", false, "show query log of dns cache")
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

	// setup a log func
	if conf.Verbose {
		log.SetFlags(conf.LogFlags)
		log.F = log.Debugf
	}

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
	app := os.Args[0]
	w := flag.Output()

	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "%s %s usage:\n", app, version)
	flag.PrintDefaults()
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Available schemes:\n")
	fmt.Fprintf(w, "  listen: mixed ss socks5 http vless trojan trojanc redir redir6 tproxy tcp udp tls ws wss unix smux kcp pxyproto\n")
	fmt.Fprintf(w, "  forward: direct reject ss socks4 socks5 http ssr ssh vless vmess trojan trojanc tcp udp tls ws wss unix smux kcp simple-obfs\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Socks5 scheme:\n")
	fmt.Fprintf(w, "  socks://[user:pass@]host:port\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "SS scheme:\n")
	fmt.Fprintf(w, "  ss://method:pass@host:port\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Available methods for ss:\n")
	fmt.Fprintf(w, "  AEAD Ciphers:\n")
	fmt.Fprintf(w, "    AEAD_AES_128_GCM AEAD_AES_192_GCM AEAD_AES_256_GCM AEAD_CHACHA20_POLY1305 AEAD_XCHACHA20_POLY1305\n")
	fmt.Fprintf(w, "  Stream Ciphers:\n")
	fmt.Fprintf(w, "    AES-128-CFB AES-128-CTR AES-192-CFB AES-192-CTR AES-256-CFB AES-256-CTR CHACHA20-IETF XCHACHA20 CHACHA20 RC4-MD5\n")
	fmt.Fprintf(w, "  Alias:\n")
	fmt.Fprintf(w, "    chacha20-ietf-poly1305 = AEAD_CHACHA20_POLY1305, xchacha20-ietf-poly1305 = AEAD_XCHACHA20_POLY1305\n")
	fmt.Fprintf(w, "  Plain: NONE\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "SSR scheme:\n")
	fmt.Fprintf(w, "  ssr://method:pass@host:port?protocol=xxx&protocol_param=yyy&obfs=zzz&obfs_param=xyz\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "SSH scheme:\n")
	fmt.Fprintf(w, "  ssh://user[:pass]@host:port[?key=keypath]\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "VMess scheme:\n")
	fmt.Fprintf(w, "  vmess://[security:]uuid@host:port[?alterID=num]\n")
	fmt.Fprintf(w, "    if alterID=0 or not set, VMessAEAD will be enabled\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "VLESS scheme:\n")
	fmt.Fprintf(w, "  vless://uuid@host:port[?fallback=127.0.0.1:80]\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Trojan client scheme:\n")
	fmt.Fprintf(w, "  trojan://pass@host:port[?serverName=SERVERNAME][&skipVerify=true][&cert=PATH]\n")
	fmt.Fprintf(w, "  trojanc://pass@host:port     (cleartext, without TLS)\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Trojan server scheme:\n")
	fmt.Fprintf(w, "  trojan://pass@host:port?cert=PATH&key=PATH[&fallback=127.0.0.1]\n")
	fmt.Fprintf(w, "  trojanc://pass@host:port[?fallback=127.0.0.1]     (cleartext, without TLS)\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Available securities for vmess:\n")
	fmt.Fprintf(w, "  none, aes-128-gcm, chacha20-poly1305\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "TLS client scheme:\n")
	fmt.Fprintf(w, "  tls://host:port[?serverName=SERVERNAME][&skipVerify=true][&cert=PATH][&alpn=proto1][&alpn=proto2]\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Proxy over tls client:\n")
	fmt.Fprintf(w, "  tls://host:port[?skipVerify=true][&serverName=SERVERNAME],scheme://\n")
	fmt.Fprintf(w, "  tls://host:port[?skipVerify=true],http://[user:pass@]\n")
	fmt.Fprintf(w, "  tls://host:port[?skipVerify=true],socks5://[user:pass@]\n")
	fmt.Fprintf(w, "  tls://host:port[?skipVerify=true],vmess://[security:]uuid@?alterID=num\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "TLS server scheme:\n")
	fmt.Fprintf(w, "  tls://host:port?cert=PATH&key=PATH[&alpn=proto1][&alpn=proto2]\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Proxy over tls server:\n")
	fmt.Fprintf(w, "  tls://host:port?cert=PATH&key=PATH,scheme://\n")
	fmt.Fprintf(w, "  tls://host:port?cert=PATH&key=PATH,http://\n")
	fmt.Fprintf(w, "  tls://host:port?cert=PATH&key=PATH,socks5://\n")
	fmt.Fprintf(w, "  tls://host:port?cert=PATH&key=PATH,ss://method:pass@\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Websocket client scheme:\n")
	fmt.Fprintf(w, "  ws://host:port[/path][?host=HOST][&origin=ORIGIN]\n")
	fmt.Fprintf(w, "  wss://host:port[/path][?serverName=SERVERNAME][&skipVerify=true][&cert=PATH][&host=HOST][&origin=ORIGIN]\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Websocket server scheme:\n")
	fmt.Fprintf(w, "  ws://:port[/path][?host=HOST]\n")
	fmt.Fprintf(w, "  wss://:port[/path]?cert=PATH&key=PATH[?host=HOST]\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Websocket with a specified proxy protocol:\n")
	fmt.Fprintf(w, "  ws://host:port[/path][?host=HOST],scheme://\n")
	fmt.Fprintf(w, "  ws://host:port[/path][?host=HOST],http://[user:pass@]\n")
	fmt.Fprintf(w, "  ws://host:port[/path][?host=HOST],socks5://[user:pass@]\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "TLS and Websocket with a specified proxy protocol:\n")
	fmt.Fprintf(w, "  tls://host:port[?skipVerify=true][&serverName=SERVERNAME],ws://[@/path[?host=HOST]],scheme://\n")
	fmt.Fprintf(w, "  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],http://[user:pass@]\n")
	fmt.Fprintf(w, "  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],socks5://[user:pass@]\n")
	fmt.Fprintf(w, "  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],vmess://[security:]uuid@?alterID=num\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Unix domain socket scheme:\n")
	fmt.Fprintf(w, "  unix://path\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Smux scheme:\n")
	fmt.Fprintf(w, "  smux://host:port\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "KCP scheme:\n")
	fmt.Fprintf(w, "  kcp://CRYPT:KEY@host:port[?dataShards=NUM&parityShards=NUM&mode=MODE]\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Available crypt types for KCP:\n")
	fmt.Fprintf(w, "  none, sm4, tea, xor, aes, aes-128, aes-192, blowfish, twofish, cast5, 3des, xtea, salsa20\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Available modes for KCP:\n")
	fmt.Fprintf(w, "  fast, fast2, fast3, normal, default: fast\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Simple-Obfs scheme:\n")
	fmt.Fprintf(w, "  simple-obfs://host:port[?type=TYPE&host=HOST&uri=URI&ua=UA]\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Available types for simple-obfs:\n")
	fmt.Fprintf(w, "  http, tls\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "DNS forwarding server:\n")
	fmt.Fprintf(w, "  dns=:53\n")
	fmt.Fprintf(w, "  dnsserver=8.8.8.8:53\n")
	fmt.Fprintf(w, "  dnsserver=1.1.1.1:53\n")
	fmt.Fprintf(w, "  dnsrecord=www.example.com/1.2.3.4\n")
	fmt.Fprintf(w, "  dnsrecord=www.example.com/2606:2800:220:1:248:1893:25c8:1946\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Available forward strategies:\n")
	fmt.Fprintf(w, "  rr: Round Robin mode\n")
	fmt.Fprintf(w, "  ha: High Availability mode\n")
	fmt.Fprintf(w, "  lha: Latency based High Availability mode\n")
	fmt.Fprintf(w, "  dh: Destination Hashing mode\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Forwarder option scheme: FORWARD_URL#OPTIONS\n")
	fmt.Fprintf(w, "  priority: set the priority of that forwarder, default:0\n")
	fmt.Fprintf(w, "  interface: set local interface or ip address used to connect remote server\n")
	fmt.Fprintf(w, "  -\n")
	fmt.Fprintf(w, "  Examples:\n")
	fmt.Fprintf(w, "    socks5://1.1.1.1:1080#priority=100\n")
	fmt.Fprintf(w, "    vmess://[security:]uuid@host:port?alterID=num#priority=200\n")
	fmt.Fprintf(w, "    vmess://[security:]uuid@host:port?alterID=num#priority=200&interface=192.168.1.99\n")
	fmt.Fprintf(w, "    vmess://[security:]uuid@host:port?alterID=num#priority=200&interface=eth0\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Services:\n")
	fmt.Fprintf(w, "  dhcpd: service=dhcpd,INTERFACE,START_IP,END_IP,LEASE_MINUTES[,MAC=IP,MAC=IP...]\n")
	fmt.Fprintf(w, "    e.g.,service=dhcpd,eth1,192.168.1.100,192.168.1.199,720\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Config file format(see `"+app+".conf.example` as an example):\n")
	fmt.Fprintf(w, "  # COMMENT LINE\n")
	fmt.Fprintf(w, "  KEY=VALUE\n")
	fmt.Fprintf(w, "  KEY=VALUE\n")
	fmt.Fprintf(w, "  # KEY equals to command line flag name: listen forward strategy...\n")
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Examples:\n")
	fmt.Fprintf(w, "  "+app+" -config glider.conf\n")
	fmt.Fprintf(w, "    -run glider with specified config file.\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  "+app+" -listen :8443 -verbose\n")
	fmt.Fprintf(w, "    -listen on :8443, serve as http/socks5 proxy on the same port, in verbose mode.\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  "+app+" -listen ss://AEAD_AES_128_GCM:pass@:8443 -verbose\n")
	fmt.Fprintf(w, "    -listen on 0.0.0.0:8443 as a ss server.\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  "+app+" -listen tls://:443?cert=crtFilePath&key=keyFilePath,http:// -verbose\n")
	fmt.Fprintf(w, "    -listen on :443 as a https(http over tls) proxy server.\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  "+app+" -listen http://:8080 -forward socks5://127.0.0.1:1080\n")
	fmt.Fprintf(w, "    -listen on :8080 as a http proxy server, forward all requests via socks5 server.\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  "+app+" -listen socks5://:1080 -forward \"tls://abc.com:443,vmess://security:uuid@?alterID=10\"\n")
	fmt.Fprintf(w, "    -listen on :1080 as a socks5 server, forward all requests via remote tls+vmess server.\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  "+app+" -listen socks5://:1080 -forward ss://method:pass@server1:port1 -forward ss://method:pass@server2:port2 -strategy rr\n")
	fmt.Fprintf(w, "    -listen on :1080 as socks5 server, forward requests via server1 and server2 in round robin mode.\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  "+app+" -listen tcp://:80 -forward tcp://2.2.2.2:80\n")
	fmt.Fprintf(w, "    -tcp tunnel: listen on :80 and forward all requests to 2.2.2.2:80.\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  "+app+" -listen udp://:53 -forward ss://method:pass@1.1.1.1:8443,udp://8.8.8.8:53\n")
	fmt.Fprintf(w, "    -listen on :53 and forward all udp requests to 8.8.8.8:53 via remote ss server.\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  "+app+" -listen socks5://:1080 -listen http://:8080 -forward ss://method:pass@1.1.1.1:8443\n")
	fmt.Fprintf(w, "    -listen on :1080 as socks5 server, :8080 as http proxy server, forward all requests via remote ss server.\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  "+app+" -verbose -listen -dns=:53 -dnsserver=8.8.8.8:53 -forward ss://method:pass@server:port -dnsrecord=www.example.com/1.2.3.4\n")
	fmt.Fprintf(w, "    -listen on :53 as dns server, forward to 8.8.8.8:53 via ss server.\n")
	fmt.Fprintf(w, "\n")
}
