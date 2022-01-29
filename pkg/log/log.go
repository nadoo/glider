package log

import (
	"fmt"
	stdlog "log"
)

var enable = false

// Set sets the logger's verbose mode and output flags.
func Set(verbose bool, flag int) {
	enable = verbose
	stdlog.SetFlags(flag)
}

// F prints debug log.
func F(f string, v ...any) {
	if enable {
		stdlog.Output(2, fmt.Sprintf(f, v...))
	}
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
