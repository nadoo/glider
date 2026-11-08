package rule

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/nadoo/conflag"
)

// Config is config of rule.
type Config struct {
	RulePath string

	Forward  []string
	Strategy Strategy

	DNSServers []string
	IPSet      string
	NFTSet     string

	Domain []string
	IP     []string
	CIDR   []string
}

// Strategy configurations.
type Strategy struct {
	Strategy            string
	Check               string
	CheckInterval       int
	CheckTimeout        int
	CheckTolerance      int
	CheckLatencySamples int
	CheckDisabledOnly   bool
	MaxFailures         int
	DialTimeout         int
	RelayTimeout        int
	IntFace             string
	Cascade             map[uint32]RetryOptions
}

// NewConfFromFile returns a new config from file.
func NewConfFromFile(ruleFile string) (*Config, error) {
	p := &Config{RulePath: ruleFile}

	f := conflag.NewFromFile("rule", ruleFile)
	f.StringSliceUniqVar(&p.Forward, "forward", nil, "forward url, format: SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS[,SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS]")
	f.StringVar(&p.Strategy.Strategy, "strategy", "rr", "forward strategy, default: rr")
	f.StringVar(&p.Strategy.Check, "check", "http://www.msftconnecttest.com/connecttest.txt#expect=200", "check=tcp[://HOST:PORT]: tcp port connect check\ncheck=http://HOST[:PORT][/URI][#expect=STRING_IN_RESP_LINE]\ncheck=file://SCRIPT_PATH: run a check script, healthy when exitcode=0, environment variables: FORWARDER_ADDR\ncheck=disable: disable health check")
	f.IntVar(&p.Strategy.CheckInterval, "checkinterval", 30, "fowarder check interval(seconds)")
	f.IntVar(&p.Strategy.CheckTimeout, "checktimeout", 10, "fowarder check timeout(seconds)")
	f.IntVar(&p.Strategy.CheckLatencySamples, "checklatencysamples", 10, "use the average latency of the latest N checks")
	f.IntVar(&p.Strategy.CheckTolerance, "checktolerance", 0, "fowarder check tolerance(ms), switch only when new_latency < old_latency - tolerance, only used in lha mode")
	f.BoolVar(&p.Strategy.CheckDisabledOnly, "checkdisabledonly", false, "check disabled fowarders only")
	f.IntVar(&p.Strategy.MaxFailures, "maxfailures", 3, "max failures to change forwarder status to disabled")
	f.IntVar(&p.Strategy.DialTimeout, "dialtimeout", 3, "dial timeout(seconds)")
	f.IntVar(&p.Strategy.RelayTimeout, "relaytimeout", 0, "relay timeout(seconds)")
	f.StringVar(&p.Strategy.IntFace, "interface", "", "source ip or source interface")

	f.StringSliceUniqVar(&p.DNSServers, "dnsserver", nil, "remote dns server")
	f.StringVar(&p.IPSet, "ipset", "", "ipset NAME, will create 2 sets: NAME for ipv4 and NAME6 for ipv6")
	f.StringVar(&p.NFTSet, "nftset", "", "nftables set [[FAMILY/]TABLE/]NAME, defaults to FAMILY=inet and TABLE=glider; will create NAME for ipv4 and NAME6 for ipv6")

	f.StringSliceVar(&p.Domain, "domain", nil, "domain")
	f.StringSliceVar(&p.IP, "ip", nil, "ip")
	f.StringSliceVar(&p.CIDR, "cidr", nil, "cidr")

	err := f.Parse()
	if err != nil {
		return nil, err
	}

	return p, err
}

type RetryOptions struct {
	Attempts     *int
	Retries      *int
	RetryDelay   *int
	AttemptDelay *int
}

func parseRetryOptions(query url.Values) (RetryOptions, error) {
	var options RetryOptions
	for _, field := range []struct {
		key      string
		min, max int
		dest     **int
	}{
		{"attempts", 1, 100, &options.Attempts},
		{"attemptdelay", 0, 5000, &options.AttemptDelay},
		{"retries", 0, 100, &options.Retries},
		{"retrydelay", 0, 5000, &options.RetryDelay},
	} {
		values, exists := query[field.key]
		if !exists {
			continue
		}
		if len(values) != 1 {
			return options, fmt.Errorf("%s must occur once", field.key)
		}
		n, err := strconv.Atoi(values[0])
		if err != nil || n < field.min || n > field.max {
			return options, fmt.Errorf("%s must be in %d..%d", field.key, field.min, field.max)
		}
		*field.dest = &n
	}
	return options, nil
}

func (s *Strategy) ParseCascade(raw []string) error {
	levels := make(map[uint32]url.Values)
	for _, value := range raw {
		query, err := url.ParseQuery(value)
		if err != nil {
			return err
		}
		for key := range query {
			if key != "cascade" && key != "attempts" && key != "attemptdelay" && key != "retries" && key != "retrydelay" {
				return fmt.Errorf("unknown cascade option %q", key)
			}
		}
		var level uint64
		if values, exists := query["cascade"]; exists {
			level, err = strconv.ParseUint(query.Get("cascade"), 10, 32)
			if err != nil || level == 0 || len(values) != 1 {
				return fmt.Errorf("cascade must occur once and be in 1..4294967295")
			}
		}
		delete(query, "cascade")
		if len(query) == 0 {
			return fmt.Errorf("cascade requires retries, retrydelay, attempts, or attemptdelay")
		}
		if levels[uint32(level)] == nil {
			levels[uint32(level)] = make(url.Values)
		}
		for key, values := range query {
			if _, exists := levels[uint32(level)][key]; exists {
				return fmt.Errorf("duplicate cascade option %q for selector %d", key, level)
			}
			levels[uint32(level)][key] = values
		}
	}
	options := make(map[uint32]RetryOptions)
	for level, query := range levels {
		parsed, err := parseRetryOptions(query)
		if err != nil {
			return err
		}
		options[level] = parsed
	}
	s.Cascade = options
	return nil
}

// ListDir returns file list named with suffix in dirPth.
func ListDir(dirPth string, suffix string) (files []string, err error) {
	files = make([]string, 0, 10)
	dir, err := os.ReadDir(dirPth)
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
