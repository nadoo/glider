package log

import (
	"fmt"
	stdlog "log"
)

// F is the main log function.
var F = func(string, ...any) {}

// SetFlags sets the output flags for the logger.
func SetFlags(flag int) {
	stdlog.SetFlags(flag)
}

// Debugf prints debug log.
func Debugf(f string, v ...any) {
	stdlog.Output(2, fmt.Sprintf(f, v...))
}

// Print prints log.
func Print(v ...any) {
	stdlog.Print(v...)
}

// Printf prints log.
func Printf(f string, v ...any) {
	stdlog.Printf(f, v...)
}

// Fatal log and exit.
func Fatal(v ...any) {
	stdlog.Fatal(v...)
}

// Fatalf log and exit.
func Fatalf(f string, v ...any) {
	stdlog.Fatalf(f, v...)
}
