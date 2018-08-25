package strategy

import (
	"bytes"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// forwarder slice orderd by priority
type priSlice []*proxy.Forwarder

func (p priSlice) Len() int           { return len(p) }
func (p priSlice) Less(i, j int) bool { return p[i].Priority() > p[j].Priority() }
func (p priSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// Dialer .
type Dialer struct {
	config   *Config
	fwdrs    priSlice
	valid    []*proxy.Forwarder
	mu       sync.Mutex
	index    int32
	priority uint32

	nextForwarder func(addr string) *proxy.Forwarder
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

	return newDialer(fwdrs, c)
}

// newDialer returns a new rrDialer
func newDialer(fwdrs []*proxy.Forwarder, c *Config) *Dialer {
	d := &Dialer{fwdrs: fwdrs, config: c}
	sort.Sort(d.fwdrs)

	d.mu.Lock()
	d.valid = d.fwdrs
	d.mu.Unlock()

	if strings.IndexByte(d.config.CheckWebSite, ':') == -1 {
		d.config.CheckWebSite += ":80"
	}

	switch c.Strategy {
	case "rr":
		d.nextForwarder = d.scheduleRR
		log.F("forward to remote servers in round robin mode.")
	case "ha":
		d.nextForwarder = d.scheduleHA
		log.F("forward to remote servers in high availability mode.")
	case "lha":
		d.nextForwarder = d.scheduleLHA
		log.F("forward to remote servers in latency based high availability mode.")
	case "dh":
		d.nextForwarder = d.scheduleDH
		log.F("forward to remote servers in destination hashing mode.")
	default:
		d.nextForwarder = d.scheduleRR
		log.F("not supported forward mode '%s', use round robin mode.", c.Strategy)
	}

	for _, f := range fwdrs {
		f.AddHandler(d.OnStatusChanged)
	}

	return d
}

// Addr returns forwarder's address
func (d *Dialer) Addr() string { return "STRATEGY" }

// Dial connects to the address addr on the network net
func (d *Dialer) Dial(network, addr string) (net.Conn, error) {
	return d.NextDialer(addr).Dial(network, addr)
}

// DialUDP connects to the given address
func (d *Dialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return d.NextDialer(addr).DialUDP(network, addr)
}

// NextDialer returns the next dialer
func (d *Dialer) NextDialer(dstAddr string) proxy.Dialer {
	return d.nextForwarder(dstAddr)
}

// Index returns the active forwarder's Index of rrDialer
func (d *Dialer) Index() int32 { return atomic.LoadInt32(&d.index) }

// SetIndex sets the active forwarder's Index of rrDialer
func (d *Dialer) SetIndex(p int32) { atomic.StoreInt32(&d.index, p) }

// IncIndex increase the index by 1
func (d *Dialer) IncIndex() int32 { return atomic.AddInt32(&d.index, 1) }

// Priority returns the active priority of rrDialer
func (d *Dialer) Priority() uint32 { return atomic.LoadUint32(&d.priority) }

// SetPriority sets the active priority of rrDialer
func (d *Dialer) SetPriority(p uint32) { atomic.StoreUint32(&d.priority, p) }

// OnStatusChanged will be called when fwdr's status changed
func (d *Dialer) OnStatusChanged(fwdr *proxy.Forwarder) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if fwdr.Enabled() {
		if fwdr.Priority() == d.Priority() {
			d.valid = append(d.valid, fwdr)
		} else if fwdr.Priority() > d.Priority() {
			d.SetPriority(fwdr.Priority())

			d.valid = nil
			for _, f := range d.fwdrs {
				if f.Enabled() && f.Priority() >= d.Priority() {
					d.valid = append(d.valid, f)
				}
			}
		}
	}

	if !fwdr.Enabled() {
		for i, f := range d.valid {
			if f == fwdr {
				d.valid[i], d.valid = d.valid[len(d.valid)-1], d.valid[:len(d.valid)-1]
				break
			}
		}
	}

	if len(d.valid) == 0 {
		d.valid = append(d.valid, d.fwdrs[0])
	}

}

// Check implements the Checker interface
func (d *Dialer) Check() {
	for i := 0; i < len(d.fwdrs); i++ {
		go d.check(i)
	}
}

func (d *Dialer) check(i int) {
	f := d.fwdrs[i]
	retry := 1
	buf := make([]byte, 4)

	for {
		time.Sleep(time.Duration(d.config.CheckInterval) * time.Second * time.Duration(retry>>1))

		retry <<= 1
		if retry > 16 {
			retry = 16
		}

		if f.Priority() < d.Priority() {
			continue
		}

		startTime := time.Now()
		rc, err := f.Dial("tcp", d.config.CheckWebSite)
		if err != nil {
			f.Disable()
			log.F("[check] %s(%d) -> %s, DISABLED. error in dial: %s", f.Addr(), f.Priority(), d.config.CheckWebSite, err)
			continue
		}

		rc.Write([]byte("GET / HTTP/1.0\r\n\r\n"))

		_, err = io.ReadFull(rc, buf)
		if err != nil {
			f.Disable()
			log.F("[check] %s(%d) -> %s, DISABLED. error in read: %s", f.Addr(), f.Priority(), d.config.CheckWebSite, err)
		} else if bytes.Equal([]byte("HTTP"), buf) {
			f.Enable()
			retry = 2
			readTime := time.Since(startTime)
			f.SetLatency(int64(readTime))
			log.F("[check] %s(%d) -> %s, ENABLED. connect time: %s", f.Addr(), f.Priority(), d.config.CheckWebSite, readTime.String())
		} else {
			f.Disable()
			log.F("[check] %s(%d) -> %s, DISABLED. server response: %s", f.Addr(), f.Priority(), d.config.CheckWebSite, buf)
		}

		rc.Close()
	}
}
