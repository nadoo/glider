module github.com/nadoo/glider

go 1.14

require (
	github.com/klauspost/cpuid v1.2.3 // indirect
	github.com/mzz2017/shadowsocksR v0.0.0-20200126130347-721f53a7b15a
	github.com/nadoo/conflag v0.2.3
	github.com/nadoo/go-shadowsocks2 v0.1.2
	github.com/pkg/errors v0.9.1 // indirect
	github.com/tjfoc/gmsm v1.3.0 // indirect
	github.com/xtaci/kcp-go/v5 v5.5.12
	golang.org/x/crypto v0.0.0-20200420201142-3c4aac89819a
	golang.org/x/net v0.0.0-20200421231249-e086a090c8fd // indirect
	golang.org/x/sys v0.0.0-20200420163511-1957bb5e6d1f // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace (
//	github.com/nadoo/conflag => ../conflag
//	github.com/nadoo/go-shadowsocks2 => ../go-shadowsocks2
// )
