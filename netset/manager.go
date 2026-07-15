package netset

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/nadoo/glider/rule"
)

type addressSet interface {
	AddAddr(netip.Addr) error
	AddPrefix(netip.Prefix) error
}

type setPair struct {
	label string
	ipv4  addressSet
	ipv6  addressSet
}

func (p *setPair) addAddr(ip netip.Addr) error {
	if ip.Is4() {
		return p.ipv4.AddAddr(ip)
	}
	return p.ipv6.AddAddr(ip)
}

func (p *setPair) addPrefix(prefix netip.Prefix) error {
	if prefix.Addr().Is4() {
		return p.ipv4.AddPrefix(prefix)
	}
	return p.ipv6.AddPrefix(prefix)
}

type backend interface {
	IPSetPair(name string) (*setPair, error)
	NFTSetPair(config nftSetConfig) (*setPair, error)
	Close() error
}

// Manager manages the ipset and nftables sets associated with rules.
type Manager struct {
	backend    backend
	domainSets map[string][]*setPair
	pairs      map[string]*setPair
}

// NewManager creates and populates all sets configured by rules.
func NewManager(rules []*rule.Config) (*Manager, error) {
	if !hasSetConfig(rules) {
		return nil, nil
	}

	b, err := newBackend()
	if err != nil {
		return nil, err
	}
	return newManager(rules, b)
}

func newManager(rules []*rule.Config, b backend) (*Manager, error) {
	m := &Manager{
		backend:    b,
		domainSets: make(map[string][]*setPair),
		pairs:      make(map[string]*setPair),
	}

	for _, config := range rules {
		if err := m.addRule(config); err != nil {
			return nil, errors.Join(err, b.Close())
		}
	}

	return m, nil
}

func hasSetConfig(rules []*rule.Config) bool {
	for _, config := range rules {
		if config.IPSet != "" || config.NFTSet != "" {
			return true
		}
	}
	return false
}

func (m *Manager) addRule(config *rule.Config) error {
	pairs := make([]*setPair, 0, 2)

	if config.IPSet != "" {
		pair, err := m.pair("ipset:"+config.IPSet, func() (*setPair, error) {
			return m.backend.IPSetPair(config.IPSet)
		})
		if err != nil {
			return fmt.Errorf("rule %q: initialize ipset %q: %w", config.RulePath, config.IPSet, err)
		}
		pairs = append(pairs, pair)
	}

	if config.NFTSet != "" {
		nftConfig, err := parseNFTSetConfig(config.NFTSet)
		if err != nil {
			return fmt.Errorf("rule %q: %w", config.RulePath, err)
		}
		pair, err := m.pair("nftset:"+nftConfig.key(), func() (*setPair, error) {
			return m.backend.NFTSetPair(nftConfig)
		})
		if err != nil {
			return fmt.Errorf("rule %q: initialize nftset %q: %w", config.RulePath, nftConfig.key(), err)
		}
		pairs = append(pairs, pair)
	}

	for _, item := range config.IP {
		ip, err := netip.ParseAddr(item)
		if err != nil {
			return fmt.Errorf("rule %q: parse set IP %q: %w", config.RulePath, item, err)
		}
		for _, pair := range pairs {
			if err := pair.addAddr(ip); err != nil {
				return fmt.Errorf("rule %q: add IP %q to %s: %w", config.RulePath, item, pair.label, err)
			}
		}
	}

	for _, item := range config.CIDR {
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			return fmt.Errorf("rule %q: parse set CIDR %q: %w", config.RulePath, item, err)
		}
		for _, pair := range pairs {
			if err := pair.addPrefix(prefix); err != nil {
				return fmt.Errorf("rule %q: add CIDR %q to %s: %w", config.RulePath, item, pair.label, err)
			}
		}
	}

	for _, domain := range config.Domain {
		domain = strings.ToLower(domain)
		for _, pair := range pairs {
			m.domainSets[domain] = appendPair(m.domainSets[domain], pair)
		}
	}

	return nil
}

func (m *Manager) pair(key string, create func() (*setPair, error)) (*setPair, error) {
	if pair, ok := m.pairs[key]; ok {
		return pair, nil
	}
	pair, err := create()
	if err != nil {
		return nil, err
	}
	m.pairs[key] = pair
	return pair, nil
}

func appendPair(pairs []*setPair, pair *setPair) []*setPair {
	for _, item := range pairs {
		if item == pair {
			return pairs
		}
	}
	return append(pairs, pair)
}

// AddDomainIP updates all sets associated with domain or one of its parents.
func (m *Manager) AddDomainIP(domain string, ip netip.Addr) error {
	if !ip.IsValid() {
		return errors.New("netset: invalid DNS answer address")
	}

	domain = strings.ToLower(domain)
	seen := make(map[*setPair]struct{})
	var errs []error
	for i := len(domain); i != -1; {
		i = strings.LastIndexByte(domain[:i], '.')
		for _, pair := range m.domainSets[domain[i+1:]] {
			if _, ok := seen[pair]; ok {
				continue
			}
			seen[pair] = struct{}{}
			if err := pair.addAddr(ip); err != nil {
				errs = append(errs, fmt.Errorf("add DNS answer %s for %q to %s: %w", ip, domain, pair.label, err))
			}
		}
	}
	return errors.Join(errs...)
}

// Close closes all netlink sockets owned by the manager.
func (m *Manager) Close() error {
	if m == nil || m.backend == nil {
		return nil
	}
	return m.backend.Close()
}
