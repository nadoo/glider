module github.com/nadoo/glider

go 1.13

require (
	github.com/dgryski/go-camellia v0.0.0-20191119043421-69a8a13fb23d // indirect
	github.com/klauspost/cpuid v1.2.3 // indirect
	github.com/klauspost/reedsolomon v1.9.3 // indirect
	github.com/nadoo/conflag v0.2.2
	github.com/nadoo/go-shadowsocks2 v0.1.2
	github.com/nadoo/shadowsocksR v0.1.0
	github.com/pkg/errors v0.9.1 // indirect
	github.com/templexxx/cpufeat v0.0.0-20180724012125-cef66df7f161 // indirect
	github.com/templexxx/xor v0.0.0-20191217153810-f85b25db303b // indirect
	github.com/tjfoc/gmsm v1.3.0 // indirect
	github.com/xtaci/kcp-go v5.4.20+incompatible
	github.com/xtaci/lossyconn v0.0.0-20190602105132-8df528c0c9ae // indirect
	golang.org/x/crypto v0.0.0-20200323165209-0ec3e9974c59
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e // indirect
	golang.org/x/sys v0.0.0-20200331124033-c3d80250170d // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace (
//	github.com/nadoo/conflag => ../conflag
//	github.com/nadoo/go-shadowsocks2 => ../go-shadowsocks2
// )
