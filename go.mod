module github.com/dongxinb/glider

go 1.13

require (
	github.com/dongxinb/go-shadowsocks2 v0.1.6
	github.com/klauspost/cpuid v1.2.1 // indirect
	github.com/klauspost/reedsolomon v1.9.3 // indirect
	github.com/nadoo/conflag v0.2.2
	github.com/nadoo/shadowsocksR v0.1.0
	github.com/pkg/errors v0.8.1 // indirect
	github.com/templexxx/cpufeat v0.0.0-20180724012125-cef66df7f161 // indirect
	github.com/templexxx/xor v0.0.0-20181023030647-4e92f724b73b // indirect
	github.com/tjfoc/gmsm v1.0.1 // indirect
	github.com/xtaci/kcp-go v5.4.11+incompatible
	github.com/xtaci/lossyconn v0.0.0-20190602105132-8df528c0c9ae // indirect
	golang.org/x/crypto v0.0.0-20191011191535-87dc89f01550
	golang.org/x/net v0.0.0-20191014212845-da9a3fd4c582 // indirect
	golang.org/x/sys v0.0.0-20191020212454-3e7259c5e7c2 // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
//	github.com/nadoo/conflag => ../conflag
// replace github.com/dongxinb/go-shadowsocks2 => ../go-shadowsocks2
