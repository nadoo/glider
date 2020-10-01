package service

import (
	"strings"

	"github.com/nadoo/glider/log"
)

// Service is a server that can be run.
type Service interface {
	Run(args ...string)
}

var services = make(map[string]Service)

// Register is used to register a service.
func Register(name string, s Service) {
	services[strings.ToLower(name)] = s
}

// Run runs a service.
func Run(name string, args ...string) {
	svc, ok := services[strings.ToLower(name)]
	if !ok {
		log.F("[service] unknown service name: %s", name)
		return
	}
	svc.Run(args...)
}
