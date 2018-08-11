package proxy

import (
	"strings"
)

// Forwarder is a forwarder
type Forwarder struct {
	Dialer
	addr     string
	disabled bool
	failures int
	priority int
	weight   int
	latency  int
}

// ForwarderFromURL returns a new forwarder
func ForwarderFromURL(s string) (*Forwarder, error) {
	var d Dialer
	var err error
	for _, url := range strings.Split(s, ",") {
		d, err = DialerFromURL(url, d)
		if err != nil {
			return nil, err
		}
	}

	return &Forwarder{Dialer: d}, nil
}

// NewForwarder .
func NewForwarder(dialer Dialer) *Forwarder {
	return &Forwarder{Dialer: dialer, addr: dialer.Addr()}
}

// Addr .
func (f *Forwarder) Addr() string {
	return f.addr
}

// Enable .
func (f *Forwarder) Enable(b bool) {
	f.disabled = !b
}

// Enabled .
func (f *Forwarder) Enabled() bool {
	return !f.disabled
}
