module github.com/nadoo/glider

go 1.15

require (
	github.com/insomniacslk/dhcp v0.0.0-20200922210017-67c425063dca
	github.com/mzz2017/shadowsocksR v1.0.0
	github.com/nadoo/conflag v0.2.3
	github.com/nadoo/go-shadowsocks2 v0.1.2
	github.com/nadoo/ipset v0.3.0
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/xtaci/kcp-go/v5 v5.6.1
	golang.org/x/crypto v0.0.0-20201016220609-9e8e0b390897
	golang.org/x/net v0.0.0-20201024042810-be3efd7ff127 // indirect
	golang.org/x/sys v0.0.0-20201022201747-fb209a7c41cd // indirect
	golang.org/x/tools v0.0.0-20201023174141-c8cfbd0f21e6 // indirect
	gopkg.in/check.v1 v1.0.0-20200902074654-038fdea0a05b // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace (
//	github.com/nadoo/conflag => ../conflag
//	github.com/nadoo/go-shadowsocks2 => ../go-shadowsocks2
// )
