module github.com/nadoo/glider

go 1.16

require (
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da
	github.com/dgryski/go-camellia v0.0.0-20191119043421-69a8a13fb23d
	github.com/dgryski/go-idea v0.0.0-20170306091226-d2fb45a411fb
	github.com/dgryski/go-rc2 v0.0.0-20150621095337-8a9021637152
	github.com/ebfe/rc2 v0.0.0-20131011165748-24b9757f5521 // indirect
	github.com/insomniacslk/dhcp v0.0.0-20210420161629-6bd1ce0fd305
	github.com/klauspost/cpuid/v2 v2.0.6 // indirect
	github.com/klauspost/reedsolomon v1.9.12 // indirect
	github.com/mdlayher/raw v0.0.0-20210412142147-51b895745faf // indirect
	github.com/nadoo/conflag v0.2.3
	github.com/nadoo/ipset v0.3.0
	github.com/tjfoc/gmsm v1.4.0 // indirect
	github.com/xtaci/kcp-go/v5 v5.6.1
	golang.org/x/crypto v0.0.0-20210415154028-4f45737414dc
	golang.org/x/net v0.0.0-20210420072503-d25e30425868 // indirect
	golang.org/x/sys v0.0.0-20210420072515-93ed5bcd2bfe // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace (
//	github.com/nadoo/conflag => ../conflag
// )
