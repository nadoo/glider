package log

import "log"

// Func defines a simple log function
type Func func(f string, v ...interface{})

// F is the main log function
var F Func = func(f string, v ...interface{}) {
}

// Fatal log and exit
func Fatal(v ...interface{}) {
	log.Fatal(v)
}

// Fatalf log and exit
func Fatalf(f string, v ...interface{}) {
	log.Fatalf(f, v)
}
