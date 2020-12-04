package matcher

import (
	"github.com/nadoo/glider/log"
	"strconv"
)

type IntegerMatcher map[int]struct{}

func NewIntegerMatcher(ports []string) *IntegerMatcher {
	m := make(map[int]struct{})
	for _, port := range ports {
		p, err := strconv.Atoi(port)
		if err != nil {
			log.F("invalid port: ", port)
			continue
		}
		m[p] = struct{}{}
	}
	if len(m) == 0 {
		return nil
	}
	return (*IntegerMatcher)(&m)
}

func (m IntegerMatcher) Match(t interface{}) bool {
	_, ok := m[t.(int)]
	return ok
}
