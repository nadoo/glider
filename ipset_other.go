// +build !linux

package main

import "errors"

type IPSetManager struct {
}

func NewIPSetManager(mainSet string, rules []*RuleConf) (*IPSetManager, error) {
	return nil, errors.New("ipset not supported on this os")
}

func (m *IPSetManager) AddDomainIP(domain, ip string) error {
	return errors.New("ipset not supported on this os")
}
