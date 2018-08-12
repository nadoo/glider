// +build !linux

package ipset

import (
	"errors"

	"github.com/nadoo/glider/rule"
)

// IPSetManager struct
type IPSetManager struct{}

// NewIPSetManager returns a IPSetManager
func NewIPSetManager(mainSet string, rules []*rule.Config) (*IPSetManager, error) {
	return nil, errors.New("ipset not supported on this os")
}

// AddDomainIP implements the DNSAnswerHandler function
func (m *IPSetManager) AddDomainIP(domain, ip string) error {
	return errors.New("ipset not supported on this os")
}
