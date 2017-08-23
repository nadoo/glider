package main

import "log"

// LogFunc .
type LogFunc func(f string, v ...interface{})

var logf LogFunc

func init() {
	logf = func(f string, v ...interface{}) {
		if conf.Verbose {
			log.Printf(f, v...)
		}
	}
}
