package service

import (
	"errors"
	"strings"
)

var creators = make(map[string]Creator)

// Service is a server that can be run.
type Service interface{ Run() }

// Creator is a function to create services.
type Creator func(args ...string) (Service, error)

// Register is used to register a service.
func Register(name string, c Creator) {
	creators[strings.ToLower(name)] = c
}

// New calls the registered creator to create services.
func New(s string) (Service, error) {
	args := strings.Split(s, ",")
	c, ok := creators[strings.ToLower(args[0])]
	if ok {
		return c(args[1:]...)
	}
	return nil, errors.New("unknown service name: '" + args[0] + "'")
}
