package matcher

import "github.com/nadoo/glider/log"

const (
	TCP = iota
	UDP
)

type NetworkMatcher [2]bool

func NewNetworkMatcher(networks []string) *NetworkMatcher {
	var bucket [2]bool
	for _, n := range networks {
		if n == "tcp" {
			bucket[TCP] = true
		} else if n == "udp" {
			bucket[UDP] = true
		} else {
			log.F("invalid network: ", n)
		}
	}
	return (*NetworkMatcher)(&bucket)
}

func (m NetworkMatcher) Match(t interface{}) bool {
	return m[t.(int)]
}
