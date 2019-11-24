module github.com/dongxinb/glider

go 1.13

require (
	github.com/dongxinb/go-shadowsocks2 v0.1.5
	github.com/nadoo/conflag v0.2.2
	github.com/nadoo/glider v0.9.2
	github.com/nadoo/shadowsocksR v0.1.0
	github.com/xtaci/kcp-go v5.4.11+incompatible
	golang.org/x/crypto v0.0.0-20191011191535-87dc89f01550
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace (
//	github.com/nadoo/conflag => ../conflag
//	github.com/dongxinb/go-shadowsocks2 => ../go-shadowsocks2
// )
