// +build !linux

package main

import "errors"

// IPSetManager struct
type IPSetManager struct{}

// NewIPSetManager returns a IPSetManager
func NewIPSetManager(mainSet string, rules []*RuleConf) (*IPSetManager, error) {
	return nil, errors.New("ipset not supported on this os")
}

// AddDomainIP implements the DNSAnswerHandler function
func (m *IPSetManager) AddDomainIP(domain, ip string) error {
	return errors.New("ipset not supported on this os")
}
