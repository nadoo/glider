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
		createSet(set)
	}

	// init ipset
	m := &Manager{}
	for _, r := range rules {
		if r.IPSet != "" {
			for _, domain := range r.Domain {
				m.domainSet.Store(domain, r.IPSet)
			}
			for _, ip := range r.IP {
				addToSet(r.IPSet, ip)
			}
			for _, cidr := range r.CIDR {
				addToSet(r.IPSet, cidr)
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
			addToSet(setName.(string), ip)
		}
	}

	return nil
}

func createSet(s string) {
	ipset.Create(s)
	ipset.Flush(s)
	ipset.Create(s+"6", ipset.OptIPv6())
	ipset.Flush(s + "6")
}

func addToSet(s, item string) error {
	if strings.IndexByte(item, '.') == -1 {
		return ipset.Add(s+"6", item)
	}
	return ipset.Add(s, item)
}
