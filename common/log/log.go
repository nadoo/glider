package log

import (
	"fmt"
	stdlog "log"
)

// F is the main log function.
var F = func(string, ...interface{}) {}

// Debugf prints debug log.
func Debugf(format string, v ...interface{}) {
	stdlog.SetFlags(stdlog.LstdFlags | stdlog.Lshortfile)
	stdlog.Output(2, fmt.Sprintf(format, v...))
}

// Printf prints log.
func Printf(format string, v ...interface{}) {
	stdlog.Printf(format, v...)
}

// Fatal log and exit.
func Fatal(v ...interface{}) {
	stdlog.Fatal(v...)
}

// Fatalf log and exit.
func Fatalf(f string, v ...interface{}) {
	stdlog.Fatalf(f, v...)
}
