module github.com/nadoo/glider

go 1.14

require (
	github.com/kr/pretty v0.1.0 // indirect
	github.com/mzz2017/shadowsocksR v0.0.0-20200809233203-ce9fb439e579
	github.com/nadoo/conflag v0.2.3
	github.com/nadoo/go-shadowsocks2 v0.1.2
	github.com/xtaci/kcp-go/v5 v5.5.15
	golang.org/x/crypto v0.0.0-20200728195943-123391ffb6de
	golang.org/x/tools v0.0.0-20200809012840-6f4f008689da // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace (
//	github.com/nadoo/conflag => ../conflag
//	github.com/nadoo/go-shadowsocks2 => ../go-shadowsocks2
// )
