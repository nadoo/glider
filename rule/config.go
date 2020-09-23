package rule

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/nadoo/conflag"
)

// Config of rule dialer.
type Config struct {
	Name string

	Forward        []string
	StrategyConfig StrategyConfig

	DNSServers []string
	IPSet      string

	Domain []string
	IP     []string
	CIDR   []string
}

// NewConfFromFile returns a new config from file.
func NewConfFromFile(ruleFile string) (*Config, error) {
	p := &Config{Name: ruleFile}

	f := conflag.NewFromFile("rule", ruleFile)
	f.StringSliceUniqVar(&p.Forward, "forward", nil, "forward url, format: SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS[,SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS]")
	f.StringVar(&p.StrategyConfig.Strategy, "strategy", "rr", "forward strategy, default: rr")
	f.StringVar(&p.StrategyConfig.CheckWebSite, "checkwebsite", "www.apple.com", "proxy check HTTP(NOT HTTPS) website address, format: HOST[:PORT], default port: 80")
	f.IntVar(&p.StrategyConfig.CheckInterval, "checkinterval", 30, "proxy check interval(seconds)")
	f.IntVar(&p.StrategyConfig.CheckTimeout, "checktimeout", 10, "proxy check timeout(seconds)")
	f.BoolVar(&p.StrategyConfig.CheckDisabledOnly, "checkdisabledonly", false, "check disabled fowarders only")
	f.IntVar(&p.StrategyConfig.MaxFailures, "maxfailures", 3, "max failures to change forwarder status to disabled")
	f.IntVar(&p.StrategyConfig.DialTimeout, "dialtimeout", 3, "dial timeout(seconds)")
	f.IntVar(&p.StrategyConfig.RelayTimeout, "relaytimeout", 0, "relay timeout(seconds)")
	f.StringVar(&p.StrategyConfig.IntFace, "interface", "", "source ip or source interface")

	f.StringSliceUniqVar(&p.DNSServers, "dnsserver", nil, "remote dns server")
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

// ListDir returns file list named with suffix in dirPth.
func ListDir(dirPth string, suffix string) (files []string, err error) {
	files = make([]string, 0, 10)
	dir, err := ioutil.ReadDir(dirPth)
	if err != nil {
		return nil, err
	}
	PthSep := string(os.PathSeparator)
	suffix = strings.ToLower(suffix)
	for _, fi := range dir {
		if fi.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(fi.Name()), suffix) {
			files = append(files, dirPth+PthSep+fi.Name())
		}
	}
	return files, nil
}
