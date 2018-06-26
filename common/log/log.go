package log

import "log"

// Func defines a simple log function
type Func func(f string, v ...interface{})

var F Func

func Fatal(v ...interface{}) {
	log.Fatal(v)
}

func Fatalf(f string, v ...interface{}) {
	log.Fatalf(f, v)
}
