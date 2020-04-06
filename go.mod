module github.com/nadoo/glider

go 1.14

require (
	github.com/klauspost/cpuid v1.2.3 // indirect
	github.com/mzz2017/shadowsocksR v0.0.0-20200126130347-721f53a7b15a
	github.com/nadoo/conflag v0.2.2
	github.com/nadoo/go-shadowsocks2 v0.1.2
	github.com/pkg/errors v0.9.1 // indirect
	github.com/templexxx/xor v0.0.0-20191217153810-f85b25db303b // indirect
	github.com/tjfoc/gmsm v1.3.0 // indirect
	github.com/xtaci/kcp-go v5.4.20+incompatible
	golang.org/x/crypto v0.0.0-20200403201458-baeed622b8d8
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e // indirect
	golang.org/x/sys v0.0.0-20200406113430-c6e801f48ba2 // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace (
//	github.com/nadoo/conflag => ../conflag
//	github.com/nadoo/go-shadowsocks2 => ../go-shadowsocks2
// )
