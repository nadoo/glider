package strategy

import (
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// Checker is an interface of forwarder checker
type Checker interface {
	Check()
}

// Config of strategy
type Config struct {
	Strategy      string
	CheckWebSite  string
	CheckInterval int
	MaxFailures   int
	IntFace       string
}

// NewDialer returns a new strategy dialer
func NewDialer(s []string, c *Config) proxy.Dialer {
	var fwdrs []*proxy.Forwarder
	for _, chain := range s {
		fwdr, err := proxy.ForwarderFromURL(chain, c.IntFace)
		if err != nil {
			log.Fatal(err)
		}
		fwdr.SetMaxFailures(uint32(c.MaxFailures))
		fwdrs = append(fwdrs, fwdr)
	}

	if len(fwdrs) == 0 {
		d, err := proxy.NewDirect(c.IntFace)
		if err != nil {
			log.Fatal(err)
		}
		return d
	}

	if len(fwdrs) == 1 {
		return fwdrs[0]
	}

	var dialer proxy.Dialer
	switch c.Strategy {
	case "rr":
		dialer = newRRDialer(fwdrs, c.CheckWebSite, c.CheckInterval)
		log.F("forward to remote servers in round robin mode.")
	case "ha":
		dialer = newHADialer(fwdrs, c.CheckWebSite, c.CheckInterval)
		log.F("forward to remote servers in high availability mode.")
	case "lha":
		dialer = newLHADialer(fwdrs, c.CheckWebSite, c.CheckInterval)
		log.F("forward to remote servers in latency based high availability mode.")
	case "dh":
		dialer = newDHDialer(fwdrs, c.CheckWebSite, c.CheckInterval)
		log.F("forward to remote servers in destination hashing mode.")
	default:
		log.F("not supported forward mode '%s', just use the first forward server.", c.Strategy)
		dialer = fwdrs[0]
	}

	return dialer
}
