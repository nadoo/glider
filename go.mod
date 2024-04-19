module github.com/nadoo/glider

go 1.20

require (
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da
	github.com/dgryski/go-camellia v0.0.0-20191119043421-69a8a13fb23d
	github.com/dgryski/go-idea v0.0.0-20170306091226-d2fb45a411fb
	github.com/dgryski/go-rc2 v0.0.0-20150621095337-8a9021637152
	github.com/insomniacslk/dhcp v0.0.0-20240129002554-15c9b8791914
	github.com/nadoo/conflag v0.3.1
	github.com/nadoo/ipset v0.5.0
	github.com/xtaci/kcp-go/v5 v5.6.7
	golang.org/x/crypto v0.21.0
	golang.org/x/sys v0.18.0
)

require (
	github.com/ebfe/rc2 v0.0.0-20131011165748-24b9757f5521 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.6 // indirect
	github.com/klauspost/reedsolomon v1.12.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/templexxx/cpu v0.1.0 // indirect
	github.com/templexxx/xorsimd v0.4.2 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	github.com/u-root/uio v0.0.0-20240118234441-a3c409a6018e // indirect
	golang.org/x/net v0.23.0 // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace github.com/nadoo/conflag => ../conflag
replace github.com/xtaci/kcp-go/v5 => github.com/xtaci/kcp-go/v5 v5.6.1 // Go1.20
