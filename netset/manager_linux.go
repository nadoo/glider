//go:build linux

package netset

import (
	"errors"
	"fmt"
	"net/netip"
	"syscall"

	libnetset "github.com/nadoo/netset"
)

type linuxBackend struct {
	ipset   *libnetset.IPSet
	nftsets []*libnetset.NFTSet
}

func newBackend() (backend, error) {
	return &linuxBackend{}, nil
}

func (b *linuxBackend) IPSetPair(name string) (*setPair, error) {
	if b.ipset == nil {
		client, err := libnetset.NewIPSet()
		if err != nil {
			return nil, err
		}
		b.ipset = client
	}

	if err := ensureIPSet(b.ipset, name); err != nil {
		return nil, err
	}
	if err := ensureIPSet(b.ipset, name+"6", libnetset.OptIPv6()); err != nil {
		return nil, err
	}

	return &setPair{
		label: "ipset " + name,
		ipv4:  ipSetTarget{client: b.ipset, name: name},
		ipv6:  ipSetTarget{client: b.ipset, name: name + "6"},
	}, nil
}

func ensureIPSet(client *libnetset.IPSet, name string, opts ...libnetset.Option) error {
	if err := client.Create(name, opts...); err != nil && !errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("create %q: %w", name, err)
	}
	if err := client.Flush(name); err != nil {
		return fmt.Errorf("flush %q: %w", name, err)
	}
	return nil
}

func (b *linuxBackend) NFTSetPair(config nftSetConfig) (*setPair, error) {
	ipv4, err := b.newNFTSet(config, config.name, false)
	if err != nil {
		return nil, err
	}
	ipv6, err := b.newNFTSet(config, config.name+"6", true)
	if err != nil {
		return nil, err
	}

	return &setPair{
		label: "nftset " + config.key(),
		ipv4:  nftSetTarget{set: ipv4},
		ipv6:  nftSetTarget{set: ipv6},
	}, nil
}

func (b *linuxBackend) newNFTSet(config nftSetConfig, name string, ipv6 bool) (*libnetset.NFTSet, error) {
	opts := []libnetset.Option{libnetset.OptFamily(config.family), libnetset.OptInterval()}
	if ipv6 {
		opts = append(opts, libnetset.OptIPv6())
	}

	set, err := libnetset.NewNFTSet(config.table, name, opts...)
	if err != nil {
		return nil, fmt.Errorf("open %s/%s/%s: %w", config.familyName, config.table, name, err)
	}
	b.nftsets = append(b.nftsets, set)

	if err := set.Create(); err != nil && !errors.Is(err, syscall.EEXIST) {
		return nil, fmt.Errorf("create %s/%s/%s: %w", config.familyName, config.table, name, err)
	}
	if err := set.Flush(); err != nil {
		return nil, fmt.Errorf("flush %s/%s/%s: %w", config.familyName, config.table, name, err)
	}
	return set, nil
}

func (b *linuxBackend) Close() error {
	var errs []error
	if b.ipset != nil {
		errs = append(errs, b.ipset.Close())
		b.ipset = nil
	}
	for _, set := range b.nftsets {
		errs = append(errs, set.Close())
	}
	b.nftsets = nil
	return errors.Join(errs...)
}

type ipSetTarget struct {
	client *libnetset.IPSet
	name   string
}

func (s ipSetTarget) AddAddr(ip netip.Addr) error {
	return s.client.AddAddr(s.name, ip)
}

func (s ipSetTarget) AddPrefix(prefix netip.Prefix) error {
	return s.client.AddPrefix(s.name, prefix)
}

type nftSetTarget struct {
	set *libnetset.NFTSet
}

func (s nftSetTarget) AddAddr(ip netip.Addr) error {
	return s.set.AddAddr(ip)
}

func (s nftSetTarget) AddPrefix(prefix netip.Prefix) error {
	return s.set.AddPrefix(prefix)
}
