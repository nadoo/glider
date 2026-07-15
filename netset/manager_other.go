//go:build !linux

package netset

import (
	"fmt"

	libnetset "github.com/nadoo/netset"
)

func newBackend() (backend, error) {
	return nil, fmt.Errorf("netset manager is supported only on linux: %w", libnetset.ErrUnsupported)
}
