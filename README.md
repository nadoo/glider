# glider
glider is a forward proxy with several protocols support.

## Install

	go get -u github.com/nadoo/glider

## Usage
```bash
glider v0.2 usage:
  -checkduration int
        proxy check duration(seconds) (default 30)
  -checkhost string
        proxy check address (default "www.apple.com:443")
  -config string
        config file path
  -forward value
        forward url, format: SCHEMA://[USER|METHOD:PASSWORD@][HOST]:PORT[,SCHEMA://[USER|METHOD:PASSWORD@][HOST]:PORT]
  -listen value
        listen url, format: SCHEMA://[USER|METHOD:PASSWORD@][HOST]:PORT
  -strategy string
        forward strategy, default: rr (default "rr")
  -verbose
        verbose mode

Available Schemas:
  mixed: serve as a http/socks5 proxy on the same port. (default)
  ss: ss proxy
  socks5: socks5 proxy
  http: http proxy
  redir: redirect proxy. (used on linux as a transparent proxy with iptables redirect rules)
  tcptun: a simple tcp tunnel
  dnstun: listen on udp port and forward all dns requests to remote dns server via forwarders(tcp)

Available schemas for different modes:
  listen: mixed ss socks5 http redir tcptun dnstun
  forward: ss socks5 http

Available methods for ss:
  AEAD_AES_128_GCM AEAD_AES_192_GCM AEAD_AES_256_GCM AEAD_CHACHA20_POLY1305 AES-128-CFB AES-128-CTR AES-192-CFB AES-192-CTR AES-256-CFB AES-256-CTR CHACHA20-IETF XCHACHA20

Available forward strategies:
  rr: Round Robin mode
  ha: High Availability mode

Examples:
  glider -config glider.conf
    -run glider with specified config file.

  glider -listen :8443
    -listen on :8443, serve as http/socks5 proxy on the same port.

  glider -listen ss://AEAD_CHACHA20_POLY1305:pass@:8443
    -listen on 0.0.0.0:8443 as a shadowsocks server.

  glider -listen socks5://:1080 -verbose
    -listen on :1080 as a socks5 proxy server, in verbose mode.

  glider -listen http://:8080 -forward socks5://127.0.0.1:1080
    -listen on :8080 as a http proxy server, forward all requests via socks5 server.

  glider -listen redir://:1081 -forward ss://method:pass@1.1.1.1:443
    -listen on :1081 as a transparent redirect server, forward all requests via remote ss server.

  glider -listen tcptun://:80=2.2.2.2:80 -forward ss://method:pass@1.1.1.1:443
    -listen on :80 and forward all requests to 2.2.2.2:80 via remote ss server.

  glider -listen socks5://:1080 -listen http://:8080 -forward ss://method:pass@1.1.1.1:443
    -listen on :1080 as socks5 server, :8080 as http proxy server, forward all requests via remote ss server.

  glider -listen redir://:1081 -listen dnstun://:53=8.8.8.8:53 -forward ss://method:pass@server1:port1,ss://method:pass@server2:port2
    -listen on :1081 as transparent redirect server, :53 as dns server, use forward chain: server1 -> server2.

  glider -listen socks5://:1080 -forward ss://method:pass@server1:port1 -forward ss://method:pass@server2:port2 -strategy rr
    -listen on :1080 as socks5 server, forward requests via server1 and server2 in roundrbin mode.
```

## Service
```bash
cd /etc/systemd/system/
vim glider.service
```

```bash
[Unit]
Description=glider
After=network.target

[Service]
Type=simple
ExecStartPre=/bin/mkdir -p /run/glider
ExecStartPre=/bin/chown nobody:nobody /run/glider
ExecStart=/opt/glider/glider -l redir://:7070 -l dnstun://:5353=8.8.8.8:53 -f ss://method:pass@yourhost:8443
ExecReload=/bin/kill -HUP $MAINPID
ExecStop=/bin/kill -INT $MAINPID
Restart=always
User=nobody
Group=nobody
UMask=0027

[Install]
WantedBy=multi-user.target
```

```bash
systemctl enable glider.service
systemctl start glider.service
```

## Thanks
- [go-ss2](https://github.com/shadowsocks/go-shadowsocks2): the core ss protocol support
- [gost](https://github.com/ginuerzh/gost): ideas and inspirations