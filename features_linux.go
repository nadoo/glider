package main

import (
	// comment out the protocol you don't need to make the compiled binary smaller.
	_ "github.com/nadoo/glider/proxy/redir"
	_ "github.com/nadoo/glider/proxy/unix"
)
