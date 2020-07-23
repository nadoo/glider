module github.com/nadoo/glider

go 1.14

require (
	github.com/klauspost/cpuid v1.3.1 // indirect
	github.com/klauspost/reedsolomon v1.9.9 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/mmcloughlin/avo v0.0.0-20200523190732-4439b6b2c061 // indirect
	github.com/mzz2017/shadowsocksR v0.0.0-20200722151714-4f4abd8a2d94
	github.com/nadoo/conflag v0.2.3
	github.com/nadoo/go-shadowsocks2 v0.1.2
	github.com/templexxx/cpu v0.0.7 // indirect
	github.com/tjfoc/gmsm v1.3.2 // indirect
	github.com/xtaci/kcp-go/v5 v5.5.14
	golang.org/x/crypto v0.0.0-20200709230013-948cd5f35899
	golang.org/x/net v0.0.0-20200707034311-ab3426394381 // indirect
	golang.org/x/sys v0.0.0-20200722175500-76b94024e4b6 // indirect
	golang.org/x/tools v0.0.0-20200723000907-a7c6fd066f6d // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace (
//	github.com/nadoo/conflag => ../conflag
//	github.com/nadoo/go-shadowsocks2 => ../go-shadowsocks2
// )
