package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"

	"github.com/nadoo/conflag"
)

var flag = conflag.New()

var conf struct {
	Verbose       bool
	Strategy      string
	CheckWebSite  string
	CheckDuration int
	Listen        []string
	Forward       []string
	RuleFile      []string
	RulesDir      string

	DNS       string
	DNSServer []string
	DNSRecord []string

	IPSet string

	rules []*RuleConf
}

func confInit() {
	flag.BoolVar(&conf.Verbose, "verbose", false, "verbose mode")
	flag.StringVar(&conf.Strategy, "strategy", "rr", "forward strategy, default: rr")
	flag.StringVar(&conf.CheckWebSite, "checkwebsite", "www.apple.com", "proxy check HTTP(NOT HTTPS) website address, format: HOST[:PORT], default port: 80")
	flag.IntVar(&conf.CheckDuration, "checkduration", 30, "proxy check duration(seconds)")
	flag.StringSliceUniqVar(&conf.Listen, "listen", nil, "listen url, format: SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS")
	flag.StringSliceUniqVar(&conf.Forward, "forward", nil, "forward url, format: SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS[,SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS]")
	flag.StringSliceUniqVar(&conf.RuleFile, "rulefile", nil, "rule file path")
	flag.StringVar(&conf.RulesDir, "rules-dir", "", "rule file folder")

	flag.StringVar(&conf.DNS, "dns", "", "dns forwarder server listen address")
	flag.StringSliceUniqVar(&conf.DNSServer, "dnsserver", []string{"8.8.8.8:53"}, "remote dns server")
	flag.StringSliceUniqVar(&conf.DNSRecord, "dnsrecord", nil, "custom dns record")

	flag.StringVar(&conf.IPSet, "ipset", "", "ipset name")

	flag.Usage = usage
	err := flag.Parse()
	if err != nil {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(-1)
	}

	if len(conf.Listen) == 0 && conf.DNS == "" {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "ERROR: listen url must be specified.\n")
		os.Exit(-1)
	}

	// rulefiles
	for _, ruleFile := range conf.RuleFile {
		rule, err := NewRuleConfFromFile(ruleFile)
		if err != nil {
			log.Fatal(err)
		}

		conf.rules = append(conf.rules, rule)
	}

	if conf.RulesDir != "" {
		if !path.IsAbs(conf.RulesDir) {
			conf.RulesDir = path.Join(flag.ConfDir(), conf.RulesDir)
		}
		ruleFolderFiles, _ := listDir(conf.RulesDir, ".rule")

		for _, ruleFile := range ruleFolderFiles {
			rule, err := NewRuleConfFromFile(ruleFile)
			if err != nil {
				log.Fatal(err)
			}

			conf.rules = append(conf.rules, rule)
		}
	}

}

func listDir(dirPth string, suffix string) (files []string, err error) {
	files = make([]string, 0, 10)
	dir, err := ioutil.ReadDir(dirPth)
	if err != nil {
		return nil, err
	}
	PthSep := string(os.PathSeparator)
	suffix = strings.ToUpper(suffix)
	for _, fi := range dir {
		if fi.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToUpper(fi.Name()), suffix) {
			files = append(files, dirPth+PthSep+fi.Name())
		}
	}
	return files, nil
}

// RuleConf , every ruleForwarder points to a rule file
type RuleConf struct {
	name string

	Forward       []string
	Strategy      string
	CheckWebSite  string
	CheckDuration int

	DNSServer []string
	IPSet     string

	Domain []string
	IP     []string
	CIDR   []string
}

// NewRuleConfFromFile .
func NewRuleConfFromFile(ruleFile string) (*RuleConf, error) {
	p := &RuleConf{name: ruleFile}

	f := conflag.NewFromFile("rule", ruleFile)
	f.StringSliceUniqVar(&p.Forward, "forward", nil, "forward url, format: SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS[,SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS]")
	f.StringVar(&p.Strategy, "strategy", "rr", "forward strategy, default: rr")
	f.StringVar(&p.CheckWebSite, "checkwebsite", "www.apple.com", "proxy check HTTP(NOT HTTPS) website address, format: HOST[:PORT], default port: 80")
	f.IntVar(&p.CheckDuration, "checkduration", 30, "proxy check duration(seconds)")

	f.StringSliceUniqVar(&p.DNSServer, "dnsserver", nil, "remote dns server")
	f.StringVar(&p.IPSet, "ipset", "", "ipset name")

	f.StringSliceUniqVar(&p.Domain, "domain", nil, "domain")
	f.StringSliceUniqVar(&p.IP, "ip", nil, "ip")
	f.StringSliceUniqVar(&p.CIDR, "cidr", nil, "cidr")

	err := f.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		return nil, err
	}

	return p, err
}

func usage() {
	app := os.Args[0]
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "%s v%s usage:\n", app, VERSION)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Available Schemes:\n")
	fmt.Fprintf(os.Stderr, "  mixed: serve as a http/socks5 proxy on the same port. (default)\n")
	fmt.Fprintf(os.Stderr, "  ss: ss proxy\n")
	fmt.Fprintf(os.Stderr, "  socks5: socks5 proxy\n")
	fmt.Fprintf(os.Stderr, "  http: http proxy\n")
	fmt.Fprintf(os.Stderr, "  ssr: ssr proxy\n")
	fmt.Fprintf(os.Stderr, "  vmess: vmess proxy\n")
	fmt.Fprintf(os.Stderr, "  tls: tls transport\n")
	fmt.Fprintf(os.Stderr, "  ws: websocket transport\n")
	fmt.Fprintf(os.Stderr, "  redir: redirect proxy. (used on linux as a transparent proxy with iptables redirect rules)\n")
	fmt.Fprintf(os.Stderr, "  tcptun: tcp tunnel\n")
	fmt.Fprintf(os.Stderr, "  udptun: udp tunnel\n")
	fmt.Fprintf(os.Stderr, "  uottun: udp over tcp tunnel\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Available schemes for different modes:\n")
	fmt.Fprintf(os.Stderr, "  listen: mixed ss socks5 http redir tcptun udptun uottun\n")
	fmt.Fprintf(os.Stderr, "  forward: ss socks5 http ssr vmess tls ws\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "SS scheme:\n")
	fmt.Fprintf(os.Stderr, "  ss://method:pass@host:port\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Available methods for ss:\n")
	fmt.Fprintf(os.Stderr, "  AEAD_AES_128_GCM AEAD_AES_192_GCM AEAD_AES_256_GCM AEAD_CHACHA20_POLY1305 AES-128-CFB AES-128-CTR AES-192-CFB AES-192-CTR AES-256-CFB AES-256-CTR CHACHA20-IETF XCHACHA20")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  NOTE: chacha20-ietf-poly1305 = AEAD_CHACHA20_POLY1305\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "SSR scheme:\n")
	fmt.Fprintf(os.Stderr, "  ssr://method:pass@host:port?protocol=xxx&protocol_param=yyy&obfs=zzz&obfs_param=xyz\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "VMess scheme:\n")
	fmt.Fprintf(os.Stderr, "  vmess://[security:]uuid@host:port?alterID=num\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Available securities for vmess:\n")
	fmt.Fprintf(os.Stderr, "  none, aes-128-gcm, chacha20-poly1305\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "TLS scheme:\n")
	fmt.Fprintf(os.Stderr, "  tls://host:port[?skipVerify=true]\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "TLS with a specified proxy protocol:\n")
	fmt.Fprintf(os.Stderr, "  tls://host:port[?skipVerify=true],scheme://\n")
	fmt.Fprintf(os.Stderr, "  tls://host:port[?skipVerify=true],http://[user:pass@]\n")
	fmt.Fprintf(os.Stderr, "  tls://host:port[?skipVerify=true],socks5://[user:pass@]\n")
	fmt.Fprintf(os.Stderr, "  tls://host:port[?skipVerify=true],vmess://[security:]uuid@?alterID=num\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Websocket scheme:\n")
	fmt.Fprintf(os.Stderr, "  ws://host:port[/path]\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "Websocket with a specified proxy protocol:\n")
	fmt.Fprintf(os.Stderr, "  ws://host:port[/path],scheme://\n")
	fmt.Fprintf(os.Stderr, "  ws://host:port[/path],http://[user:pass@]\n")
	fmt.Fprintf(os.Stderr, "  ws://host:port[/path],socks5://[user:pass@]\n")
	fmt.Fprintf(os.Stderr, "  ws://host:port[/path],vmess://[security:]uuid@?alterID=num\n")
	fmt.Fprintf(os.Stderr, "\n")

	fmt.Fprintf(os.Stderr, "TLS and Websocket with a specified proxy protocol:\n")
	fmt.Fprintf(os.Stderr, "  tls://host:port[?skipVerify=true],ws://[@/path],scheme://\n")
	fmt.Fprintf(os.Stderr, "  tls://host:port[?skipVerify=true],ws://[@/path],http://[user:pass@]\n")
	fmt.Fprintf(os.Stderr, "  tls://host:port[?skipVerify=true],ws://[@/path],socks5://[user:pass@]\n")
	fmt.Fprintf(os.Stderr, "  tls://host:port[?skipVerify=true],ws://[@/path],vmess://[security:]uuid@?alterID=num\n")
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
	fmt.Fprintf(os.Stderr, "  "+app+" -listen redir://:1081 -forward \"ssr://method:pass@1.1.1.1:8444?protocol=a&protocol_param=b&obfs=c&obfs_param=d\"\n")
	fmt.Fprintf(os.Stderr, "    -listen on :1081 as a transparent redirect server, forward all requests via remote ssr server.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen redir://:1081 -forward \"tls://1.1.1.1:443,vmess://security:uuid@?alterID=10\"\n")
	fmt.Fprintf(os.Stderr, "    -listen on :1081 as a transparent redirect server, forward all requests via remote tls+vmess server.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen redir://:1081 -forward \"ws://1.1.1.1:80,vmess://security:uuid@?alterID=10\"\n")
	fmt.Fprintf(os.Stderr, "    -listen on :1081 as a transparent redirect server, forward all requests via remote ws+vmess server.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen tcptun://:80=2.2.2.2:80 -forward ss://method:pass@1.1.1.1:8443\n")
	fmt.Fprintf(os.Stderr, "    -listen on :80 and forward all requests to 2.2.2.2:80 via remote ss server.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen udptun://:53=8.8.8.8:53 -forward ss://method:pass@1.1.1.1:8443\n")
	fmt.Fprintf(os.Stderr, "    -listen on :53 and forward all udp requests to 8.8.8.8:53 via remote ss server.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen uottun://:53=8.8.8.8:53 -forward ss://method:pass@1.1.1.1:8443\n")
	fmt.Fprintf(os.Stderr, "    -listen on :53 and forward all udp requests via udp over tcp tunnel.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen socks5://:1080 -listen http://:8080 -forward ss://method:pass@1.1.1.1:8443\n")
	fmt.Fprintf(os.Stderr, "    -listen on :1080 as socks5 server, :8080 as http proxy server, forward all requests via remote ss server.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen redir://:1081 -dns://:53 -dnsserver://8.8.8.8:53 -forward ss://method:pass@server1:port1,ss://method:pass@server2:port2\n")
	fmt.Fprintf(os.Stderr, "    -listen on :1081 as transparent redirect server, :53 as dns server, use forward chain: server1 -> server2.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  "+app+" -listen socks5://:1080 -forward ss://method:pass@server1:port1 -forward ss://method:pass@server2:port2 -strategy rr\n")
	fmt.Fprintf(os.Stderr, "    -listen on :1080 as socks5 server, forward requests via server1 and server2 in round robin mode.\n")
	fmt.Fprintf(os.Stderr, "\n")
}
