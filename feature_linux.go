package main

import (
	// comment out the services you don't need to make the compiled binary smaller.
	// _ "github.com/nadoo/glider/service/xxx"

	// comment out the protocols you don't need to make the compiled binary smaller.
	_ "github.com/nadoo/glider/proxy/redir"
	_ "github.com/nadoo/glider/proxy/tproxy"
	_ "github.com/nadoo/glider/proxy/unix"
)
