module github.com/nadoo/glider

go 1.15

require (
	github.com/mzz2017/shadowsocksR v0.0.0-20200809233203-ce9fb439e579
	github.com/nadoo/conflag v0.2.3
	github.com/nadoo/go-shadowsocks2 v0.1.2
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/xtaci/kcp-go/v5 v5.5.15
	golang.org/x/crypto v0.0.0-20200728195943-123391ffb6de
	golang.org/x/sys v0.0.0-20200812155832-6a926be9bd1d // indirect
	golang.org/x/tools v0.0.0-20200812231640-9176cd30088c // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
)

// Replace dependency modules with local developing copy
// use `go list -m all` to confirm the final module used
// replace (
//	github.com/nadoo/conflag => ../conflag
//	github.com/nadoo/go-shadowsocks2 => ../go-shadowsocks2
// )
