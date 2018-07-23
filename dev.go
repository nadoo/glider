//+build dev

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go func() {
		err := http.ListenAndServe(":6060", nil)
		if err != nil {
			fmt.Printf("Create pprof server error: %s\n", err)
		}
	}()
}
