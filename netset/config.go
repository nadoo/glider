package netset

import (
	"fmt"
	"strings"

	libnetset "github.com/nadoo/netset"
)

const (
	defaultNFTFamily = "inet"
	defaultNFTTable  = "glider"
	maxNFTNameLength = 256
)

type nftSetConfig struct {
	family     libnetset.Family
	familyName string
	table      string
	name       string
}

func parseNFTSetConfig(value string) (nftSetConfig, error) {
	parts := strings.Split(strings.TrimSpace(value), "/")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	config := nftSetConfig{
		family:     libnetset.FamilyINet,
		familyName: defaultNFTFamily,
		table:      defaultNFTTable,
	}

	switch len(parts) {
	case 1:
		config.name = parts[0]
	case 2:
		config.table = parts[0]
		config.name = parts[1]
	case 3:
		config.familyName = parts[0]
		config.table = parts[1]
		config.name = parts[2]
	default:
		return nftSetConfig{}, fmt.Errorf("invalid nftset %q: expected [[FAMILY/]TABLE/]NAME", value)
	}

	if config.familyName == "" || config.table == "" || config.name == "" {
		return nftSetConfig{}, fmt.Errorf("invalid nftset %q: family, table, and set name must not be empty", value)
	}

	family, ok := nftFamilies[strings.ToLower(config.familyName)]
	if !ok {
		return nftSetConfig{}, fmt.Errorf("invalid nftset %q: unsupported family %q", value, config.familyName)
	}
	config.family = family
	config.familyName = strings.ToLower(config.familyName)
	if err := validateNFTName("table", config.table); err != nil {
		return nftSetConfig{}, fmt.Errorf("invalid nftset %q: %w", value, err)
	}
	if err := validateNFTName("set", config.name); err != nil {
		return nftSetConfig{}, fmt.Errorf("invalid nftset %q: %w", value, err)
	}
	if err := validateNFTName("IPv6 set", config.name+"6"); err != nil {
		return nftSetConfig{}, fmt.Errorf("invalid nftset %q: %w", value, err)
	}

	return config, nil
}

func validateNFTName(kind, name string) error {
	if strings.IndexByte(name, 0) >= 0 {
		return fmt.Errorf("%s name must not contain NUL", kind)
	}
	if len(name) >= maxNFTNameLength {
		return fmt.Errorf("%s name is too long", kind)
	}
	return nil
}

func (c nftSetConfig) key() string {
	return c.familyName + "/" + c.table + "/" + c.name
}

var nftFamilies = map[string]libnetset.Family{
	"inet":   libnetset.FamilyINet,
	"ip":     libnetset.FamilyIPv4,
	"ip6":    libnetset.FamilyIPv6,
	"arp":    libnetset.FamilyARP,
	"netdev": libnetset.FamilyNetdev,
	"bridge": libnetset.FamilyBridge,
}
