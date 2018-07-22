//+build dev

package main

import (
	"net/http"
	_ "net/http/pprof"

	_ "github.com/nadoo/glider/proxy/ws"
)

func init() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
}
