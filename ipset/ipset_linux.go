package ipset

import (
	"errors"
	"strings"
	"sync"

	"github.com/nadoo/ipset"

	"github.com/nadoo/glider/rule"
)

// Manager struct.
type Manager struct {
	domainSet sync.Map
}

// NewManager returns a Manager
func NewManager(rules []*rule.Config) (*Manager, error) {
	if err := ipset.Init(); err != nil {
		return nil, err
	}

	// create ipset, avoid redundant.
	sets := make(map[string]struct{})
	for _, r := range rules {
		if r.IPSet != "" {
			sets[r.IPSet] = struct{}{}
		}
	}

	for set := range sets {
		ipset.Create(set)
		ipset.Flush(set)
	}

	// init ipset
	m := &Manager{}
	for _, r := range rules {
		if r.IPSet != "" {
			for _, domain := range r.Domain {
				m.domainSet.Store(domain, r.IPSet)
			}
			for _, ip := range r.IP {
				ipset.Add(r.IPSet, ip)
			}
			for _, cidr := range r.CIDR {
				ipset.Add(r.IPSet, cidr)
			}
		}
	}

	return m, nil
}

// AddDomainIP implements the dns AnswerHandler function, used to update ipset according to domainSet rule.
func (m *Manager) AddDomainIP(domain, ip string) error {
	if domain == "" || ip == "" {
		return errors.New("please specify the domain and ip address")
	}

	domain = strings.ToLower(domain)
	for i := len(domain); i != -1; {
		i = strings.LastIndexByte(domain[:i], '.')
		if setName, ok := m.domainSet.Load(domain[i+1:]); ok {
			ipset.Add(setName.(string), ip)
		}
	}

	return nil
}
