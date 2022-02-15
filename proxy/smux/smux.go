package smux

import "github.com/nadoo/glider/proxy"

func init() {
	proxy.AddUsage("smux", `
Smux scheme:
  smux://host:port
`)
}
