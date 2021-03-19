module github.com/nadoo/glider

go 1.16

require (
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da
	github.com/dgryski/go-camellia v0.0.0-20191119043421-69a8a13fb23d
	github.com/dgryski/go-idea v0.0.0-20170306091226-d2fb45a411fb
	github.com/dgryski/go-rc2 v0.0.0-20150621095337-8a9021637152
	github.com/ebfe/rc2 v0.0.0-20131011165748-24b9757f5521 // indirect
	github.com/insomniacslk/dhcp v0.0.0-20210315110227-c51060810aaa
	github.com/klauspost/cpuid/v2 v2.0.5 // indirect
	github.com/klauspost/reedsolomon v1.9.12 // indirect
	github.com/nadoo/conflag v0.2.3
	github.com/nadoo/ipset v0.3.0
	github.com/tjfoc/gmsm v1.4.0 // indirect
	github.com/xtaci/kcp-go/v5 v5.6.1
	golang.org/x/crypto v0.0.0-20210317152858-513c2a44f670
	golang.org/x/net v0.0.0-20210316092652-d523dce5a7f4 // indirect
	golang.org/x/sys v0.0.0-20210319071255-635bc2c9138d // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace (
//	github.com/nadoo/conflag => ../conflag
// )
