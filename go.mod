module github.com/nadoo/glider

go 1.12

require (
	github.com/Yawning/chacha20 v0.0.0-20170904085104-e3b1f968fc63 // indirect
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da // indirect
	github.com/dgryski/go-camellia v0.0.0-20140412174459-3be6b3054dd1 // indirect
	github.com/dgryski/go-idea v0.0.0-20170306091226-d2fb45a411fb // indirect
	github.com/dgryski/go-rc2 v0.0.0-20150621095337-8a9021637152 // indirect
	github.com/ebfe/rc2 v0.0.0-20131011165748-24b9757f5521 // indirect
	github.com/klauspost/cpuid v1.2.0 // indirect
	github.com/klauspost/reedsolomon v1.9.1 // indirect
	github.com/nadoo/conflag v0.2.0
	github.com/nadoo/go-shadowsocks2 v0.1.0
	github.com/pkg/errors v0.8.1 // indirect
	github.com/sun8911879/shadowsocksR v0.0.0-20180529042039-da20fda4804f
	github.com/templexxx/cpufeat v0.0.0-20180724012125-cef66df7f161 // indirect
	github.com/templexxx/xor v0.0.0-20181023030647-4e92f724b73b // indirect
	github.com/tjfoc/gmsm v1.0.1 // indirect
	github.com/xtaci/kcp-go v5.0.7+incompatible
	golang.org/x/crypto v0.0.0-20190320223903-b7391e95e576
	golang.org/x/net v0.0.0-20190320064053-1272bf9dcd53 // indirect
	golang.org/x/sys v0.0.0-20190321052220-f7bb7a8bee54 // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace (
//	github.com/nadoo/conflag => ../conflag
//	github.com/nadoo/go-shadowsocks2 => ../go-shadowsocks2
// )
