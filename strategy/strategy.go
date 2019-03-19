package strategy

import (
	"bytes"
	"hash/fnv"
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

// Checker is an interface of forwarder checker.
type Checker interface {
	Check()
}

// Config is strategy config struct.
type Config struct {
	Strategy      string
	CheckWebSite  string
	CheckInterval int
	CheckTimeout  int
	MaxFailures   int
	IntFace       string
}

// forwarder slice orderd by priority
type priSlice []*proxy.Forwarder

func (p priSlice) Len() int           { return len(p) }
func (p priSlice) Less(i, j int) bool { return p[i].Priority() > p[j].Priority() }
func (p priSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// Dialer is base dialer struct.
type Dialer struct {
	config    *Config
	fwdrs     priSlice
	available []*proxy.Forwarder
	mu        sync.RWMutex
	index     uint32
	priority  uint32

	nextForwarder func(addr string) *proxy.Forwarder
}

// NewDialer returns a new strategy dialer.
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

	d.initAvailable()

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
		f.AddHandler(d.onStatusChanged)
	}

	return d
}

// Addr returns forwarder's address.
func (d *Dialer) Addr() string { return "STRATEGY" }

// Dial connects to the address addr on the network net.
func (d *Dialer) Dial(network, addr string) (net.Conn, error) {
	return d.NextDialer(addr).Dial(network, addr)
}

// DialUDP connects to the given address.
func (d *Dialer) DialUDP(network, addr string) (pc net.PacketConn, writeTo net.Addr, err error) {
	return d.NextDialer(addr).DialUDP(network, addr)
}

// NextDialer returns the next dialer.
func (d *Dialer) NextDialer(dstAddr string) proxy.Dialer {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.nextForwarder(dstAddr)
}

// Priority returns the active priority of dialer.
func (d *Dialer) Priority() uint32 { return atomic.LoadUint32(&d.priority) }

// SetPriority sets the active priority of daler.
func (d *Dialer) SetPriority(p uint32) { atomic.StoreUint32(&d.priority, p) }

// initAvailable traverse d.fwdrs and init the available forwarder slice.
func (d *Dialer) initAvailable() {
	for _, f := range d.fwdrs {
		if f.Enabled() {
			d.SetPriority(f.Priority())
			break
		}
	}

	d.available = nil
	for _, f := range d.fwdrs {
		if f.Enabled() && f.Priority() >= d.Priority() {
			d.available = append(d.available, f)
		}
	}

	if len(d.available) == 0 {
		// no available forwarders, set priority to 0 to check all forwarders in check func
		d.SetPriority(0)
		log.F("[strategy] no available forwarders, just use: %s, please check your settings or network", d.fwdrs[0].Addr())
		d.available = append(d.available, d.fwdrs[0])
	}
}

// onStatusChanged will be called when fwdr's status changed.
func (d *Dialer) onStatusChanged(fwdr *proxy.Forwarder) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if fwdr.Enabled() {
		log.F("[strategy] %s changed status from Disabled to Enabled ", fwdr.Addr())
		if fwdr.Priority() == d.Priority() {
			d.available = append(d.available, fwdr)
		} else if fwdr.Priority() > d.Priority() {
			d.initAvailable()
		}
	} else {
		log.F("[strategy] %s changed status from Enabled to Disabled", fwdr.Addr())
		for i, f := range d.available {
			if f == fwdr {
				d.available[i], d.available = d.available[len(d.available)-1], d.available[:len(d.available)-1]
				break
			}
		}
	}

	if len(d.available) == 0 {
		d.initAvailable()
	}
}

// Check implements the Checker interface.
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

		// check all forwarders at least one time
		if retry > 1 && f.Priority() < d.Priority() {
			continue
		}

		retry <<= 1
		if retry > 16 {
			retry = 16
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

			readTime := time.Since(startTime)
			f.SetLatency(int64(readTime))

			if readTime > time.Duration(d.config.CheckTimeout)*time.Second {
				f.Disable()
				log.F("[check] %s(%d) -> %s, DISABLED. check timeout: %s", f.Addr(), f.Priority(), d.config.CheckWebSite, readTime)
			} else {
				retry = 2
				f.Enable()
				log.F("[check] %s(%d) -> %s, ENABLED. connect time: %s", f.Addr(), f.Priority(), d.config.CheckWebSite, readTime)
			}

		} else {
			f.Disable()
			log.F("[check] %s(%d) -> %s, DISABLED. server response: %s", f.Addr(), f.Priority(), d.config.CheckWebSite, buf)
		}

		rc.Close()
	}
}

// Round Robin
func (d *Dialer) scheduleRR(dstAddr string) *proxy.Forwarder {
	return d.available[atomic.AddUint32(&d.index, 1)%uint32(len(d.available))]
}

// High Availability
func (d *Dialer) scheduleHA(dstAddr string) *proxy.Forwarder {
	return d.available[0]
}

// Latency based High Availability
func (d *Dialer) scheduleLHA(dstAddr string) *proxy.Forwarder {
	fwdr := d.available[0]
	lowest := fwdr.Latency()
	for _, f := range d.available {
		if f.Latency() < lowest {
			lowest = f.Latency()
			fwdr = f
		}
	}
	return fwdr
}

// Destination Hashing
func (d *Dialer) scheduleDH(dstAddr string) *proxy.Forwarder {
	fnv1a := fnv.New32a()
	fnv1a.Write([]byte(dstAddr))
	return d.available[fnv1a.Sum32()%uint32(len(d.available))]
}
