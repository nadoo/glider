# [glider](https://github.com/nadoo/glider)

[![Go Report Card](https://goreportcard.com/badge/github.com/nadoo/glider?style=flat-square)](https://goreportcard.com/report/github.com/nadoo/glider)
[![GitHub release](https://img.shields.io/github/v/release/nadoo/glider.svg?style=flat-square&include_prereleases)](https://github.com/nadoo/glider/releases)
[![Actions Status](https://img.shields.io/github/workflow/status/nadoo/glider/Build?style=flat-square)](https://github.com/nadoo/glider/actions)

glider is a forward proxy with multiple protocols support, and also a dns/dhcp server with ipset management features(like dnsmasq).

we can set up local listeners as proxy servers, and forward requests to internet via forwarders.

```bash
                |Forwarder ----------------->|
   Listener --> |                            | Internet
                |Forwarder --> Forwarder->...|
```

## Features
- Act as both proxy client and proxy server(protocol converter)
- Flexible proxy & protocol chains
- Load balancing with the following scheduling algorithm:
  - rr: round robin
  - ha: high availability 
  - lha: latency based high availability
  - dh: destination hashing
- Rule & priority based forwarder choosing: [Config Examples](config/examples)
- DNS forwarding server:
  - dns over proxy
  - force upstream querying by tcp
  - association rules between dns and forwarder choosing
  - association rules between dns and ipset
  - dns cache support
  - custom dns record
- IPSet management (linux kernel version >= 2.6.32):
  - add ip/cidrs from rule files on startup
  - add resolved ips for domains from rule files by dns forwarding server
- Serve http and socks5 on the same port
- Periodical availability checking for forwarders
- Send requests from specific local ip/interface
- Services: 
  - dhcpd: a simple dhcp server that can detect existing dhcp server and avoid conflicts

## Protocols

<details>
<summary>click to see details</summary>

|Protocol       | Listen/TCP |  Listen/UDP | Forward/TCP | Forward/UDP | Description
|:-:            |:-:|:-:|:-:|:-:|:-
|Mixed          |√|√| | |http+socks5 server
|HTTP           |√| |√| |client & server
|SOCKS5         |√|√|√|√|client & server
|SS             |√|√|√|√|client & server
|Trojan         |√|√|√|√|client & server
|Trojanc        |√|√|√|√|trojan cleartext(without tls)
|VLESS          |√|√|√|√|client & server
|VMess          | | |√|√|client only
|SSR            | | |√| |client only
|SSH            | | |√| |client only
|SOCKS4         | | |√| |client only
|SOCKS4A        | | |√| |client only
|TCP            |√| |√| |tcp tunnel client & server
|UDP            | |√| |√|udp tunnel client & server
|TLS            |√| |√| |transport client & server
|KCP            | |√|√| |transport client & server
|Unix           |√|√|√|√|transport client & server
|Smux           |√| |√| |transport client & server
|Websocket(WS)  |√| |√| |transport client & server
|WS Secure      |√| |√| |websocket secure (wss)
|Proxy Protocol |√| | | |version 1 server only
|Simple-Obfs    | | |√| |transport client only
|Redir          |√| | | |linux redirect proxy
|Redir6         |√| | | |linux redirect proxy(ipv6)
|Tproxy         | |√| | |linux tproxy(udp only)
|Reject         | | |√|√|reject all requests

</details>

## Install

Download:
- [https://github.com/nadoo/glider/releases](https://github.com/nadoo/glider/releases)

Docker:
```bash
docker pull nadoo/glider
#docker pull ghcr.io/nadoo/glider
```

ArchLinux:
```bash
sudo pacman -S glider
```

## Usage

```bash
glider -h
```
<details>
<summary>click to see details</summary>

```bash
glider 0.15.0 usage:
  -check string
    	check=tcp[://HOST:PORT]: tcp port connect check
    	check=http://HOST[:PORT][/URI][#expect=STRING_IN_RESP_LINE]
    	check=file://SCRIPT_PATH: run a check script, healthy when exitcode=0, environment variables: FORWARDER_ADDR
    	check=disable: disable health check (default "http://www.msftconnecttest.com/connecttest.txt#expect=200")
  -checkdisabledonly
    	check disabled fowarders only
  -checkinterval int
    	fowarder check interval(seconds) (default 30)
  -checktimeout int
    	fowarder check timeout(seconds) (default 10)
  -checktolerance int
    	fowarder check tolerance(ms), switch only when new_latency < old_latency - tolerance, only used in lha mode
  -config string
    	config file path
  -dialtimeout int
    	dial timeout(seconds) (default 3)
  -dns string
    	local dns server listen address
  -dnsalwaystcp
    	always use tcp to query upstream dns servers no matter there is a forwarder or not
  -dnscachelog
    	show query log of dns cache
  -dnscachesize int
    	size of CACHE (default 4096)
  -dnsmaxttl int
    	maximum TTL value for entries in the CACHE(seconds) (default 1800)
  -dnsminttl int
    	minimum TTL value for entries in the CACHE(seconds)
  -dnsrecord value
    	custom dns record, format: domain/ip
  -dnsserver value
    	remote dns server address
  -dnstimeout int
    	timeout value used in multiple dnsservers switch(seconds) (default 3)
  -forward value
    	forward url, format: SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS[,SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS]
  -include value
    	include file
  -interface string
    	source ip or source interface
  -listen value
    	listen url, format: SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS
  -logflags int
    	log flags, do not change it if you do not know what it is, ref: https://pkg.go.dev/log#pkg-constants (default 19)
  -maxfailures int
    	max failures to change forwarder status to disabled (default 3)
  -relaytimeout int
    	relay timeout(seconds)
  -rulefile value
    	rule file path
  -rules-dir string
    	rule file folder
  -service value
    	run specified services, format: SERVICE_NAME[,SERVICE_CONFIG]
  -strategy string
    	forward strategy, default: rr (default "rr")
  -tcpbufsize int
    	tcp buffer size in Bytes (default 32768)
  -udpbufsize int
    	udp buffer size in Bytes (default 2048)
  -verbose
    	verbose mode
```

</details>

run:
```bash
glider -config CONFIGPATH
```
```bash
glider -verbose -listen :8443 -forward SCHEME://HOST:PORT
```

#### Schemes

<details>
<summary>click to see details</summary>

```bash
Available schemes:
  listen: mixed ss socks5 http vless trojan trojanc redir redir6 tproxy tcp udp tls ws wss unix smux kcp pxyproto
  forward: direct reject ss socks4 socks5 http ssr ssh vless vmess trojan trojanc tcp udp tls ws wss unix smux kcp simple-obfs

Socks5 scheme:
  socks://[user:pass@]host:port

SS scheme:
  ss://method:pass@host:port

Available methods for ss:
  AEAD Ciphers:
    AEAD_AES_128_GCM AEAD_AES_192_GCM AEAD_AES_256_GCM AEAD_CHACHA20_POLY1305 AEAD_XCHACHA20_POLY1305
  Stream Ciphers:
    AES-128-CFB AES-128-CTR AES-192-CFB AES-192-CTR AES-256-CFB AES-256-CTR CHACHA20-IETF XCHACHA20 CHACHA20 RC4-MD5
  Alias:
    chacha20-ietf-poly1305 = AEAD_CHACHA20_POLY1305, xchacha20-ietf-poly1305 = AEAD_XCHACHA20_POLY1305
  Plain: NONE

SSR scheme:
  ssr://method:pass@host:port?protocol=xxx&protocol_param=yyy&obfs=zzz&obfs_param=xyz

SSH scheme:
  ssh://user[:pass]@host:port[?key=keypath]

VMess scheme:
  vmess://[security:]uuid@host:port[?alterID=num]
    if alterID=0 or not set, VMessAEAD will be enabled

VLESS scheme:
  vless://uuid@host:port[?fallback=127.0.0.1:80]

Trojan client scheme:
  trojan://pass@host:port[?serverName=SERVERNAME][&skipVerify=true][&cert=PATH]
  trojanc://pass@host:port     (cleartext, without TLS)

Trojan server scheme:
  trojan://pass@host:port?cert=PATH&key=PATH[&fallback=127.0.0.1]
  trojanc://pass@host:port[?fallback=127.0.0.1]     (cleartext, without TLS)

Available securities for vmess:
  none, aes-128-gcm, chacha20-poly1305

TLS client scheme:
  tls://host:port[?serverName=SERVERNAME][&skipVerify=true][&cert=PATH][&alpn=proto1][&alpn=proto2]

Proxy over tls client:
  tls://host:port[?skipVerify=true][&serverName=SERVERNAME],scheme://
  tls://host:port[?skipVerify=true],http://[user:pass@]
  tls://host:port[?skipVerify=true],socks5://[user:pass@]
  tls://host:port[?skipVerify=true],vmess://[security:]uuid@?alterID=num

TLS server scheme:
  tls://host:port?cert=PATH&key=PATH[&alpn=proto1][&alpn=proto2]

Proxy over tls server:
  tls://host:port?cert=PATH&key=PATH,scheme://
  tls://host:port?cert=PATH&key=PATH,http://
  tls://host:port?cert=PATH&key=PATH,socks5://
  tls://host:port?cert=PATH&key=PATH,ss://method:pass@

Websocket client scheme:
  ws://host:port[/path][?host=HOST][&origin=ORIGIN]
  wss://host:port[/path][?serverName=SERVERNAME][&skipVerify=true][&cert=PATH][&host=HOST][&origin=ORIGIN]

Websocket server scheme:
  ws://:port[/path][?host=HOST]
  wss://:port[/path]?cert=PATH&key=PATH[?host=HOST]

Websocket with a specified proxy protocol:
  ws://host:port[/path][?host=HOST],scheme://
  ws://host:port[/path][?host=HOST],http://[user:pass@]
  ws://host:port[/path][?host=HOST],socks5://[user:pass@]

TLS and Websocket with a specified proxy protocol:
  tls://host:port[?skipVerify=true][&serverName=SERVERNAME],ws://[@/path[?host=HOST]],scheme://
  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],http://[user:pass@]
  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],socks5://[user:pass@]
  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],vmess://[security:]uuid@?alterID=num

Unix domain socket scheme:
  unix://path

Smux scheme:
  smux://host:port

KCP scheme:
  kcp://CRYPT:KEY@host:port[?dataShards=NUM&parityShards=NUM&mode=MODE]

Available crypt types for KCP:
  none, sm4, tea, xor, aes, aes-128, aes-192, blowfish, twofish, cast5, 3des, xtea, salsa20

Available modes for KCP:
  fast, fast2, fast3, normal, default: fast

Simple-Obfs scheme:
  simple-obfs://host:port[?type=TYPE&host=HOST&uri=URI&ua=UA]

Available types for simple-obfs:
  http, tls

DNS forwarding server:
  dns=:53
  dnsserver=8.8.8.8:53
  dnsserver=1.1.1.1:53
  dnsrecord=www.example.com/1.2.3.4
  dnsrecord=www.example.com/2606:2800:220:1:248:1893:25c8:1946

Available forward strategies:
  rr: Round Robin mode
  ha: High Availability mode
  lha: Latency based High Availability mode
  dh: Destination Hashing mode

Forwarder option scheme: FORWARD_URL#OPTIONS
  priority: set the priority of that forwarder, default:0
  interface: set local interface or ip address used to connect remote server
  -
  Examples:
    socks5://1.1.1.1:1080#priority=100
    vmess://[security:]uuid@host:port?alterID=num#priority=200
    vmess://[security:]uuid@host:port?alterID=num#priority=200&interface=192.168.1.99
    vmess://[security:]uuid@host:port?alterID=num#priority=200&interface=eth0

Services:
  dhcpd: service=dhcpd,INTERFACE,START_IP,END_IP,LEASE_MINUTES[,MAC=IP,MAC=IP...]
    e.g.,service=dhcpd,eth1,192.168.1.100,192.168.1.199,720

Config file format(see `./glider.conf.example` as an example):
  # COMMENT LINE
  KEY=VALUE
  KEY=VALUE
  # KEY equals to command line flag name: listen forward strategy...
```

</details>

#### Examples

<details>
<summary>click to see details</summary>

```bash
Examples:
  glider -config glider.conf
    -run glider with specified config file.

  glider -listen :8443 -verbose
    -listen on :8443, serve as http/socks5 proxy on the same port, in verbose mode.

  glider -listen ss://AEAD_AES_128_GCM:pass@:8443 -verbose
    -listen on 0.0.0.0:8443 as a ss server.

  glider -listen tls://:443?cert=crtFilePath&key=keyFilePath,http:// -verbose
    -listen on :443 as a https(http over tls) proxy server.

  glider -listen http://:8080 -forward socks5://127.0.0.1:1080
    -listen on :8080 as a http proxy server, forward all requests via socks5 server.

  glider -listen socks5://:1080 -forward "tls://abc.com:443,vmess://security:uuid@?alterID=10"
    -listen on :1080 as a socks5 server, forward all requests via remote tls+vmess server.

  glider -listen socks5://:1080 -forward ss://method:pass@server1:port1 -forward ss://method:pass@server2:port2 -strategy rr
    -listen on :1080 as socks5 server, forward requests via server1 and server2 in round robin mode.

  glider -listen tcp://:80 -forward tcp://2.2.2.2:80
    -tcp tunnel: listen on :80 and forward all requests to 2.2.2.2:80.

  glider -listen udp://:53 -forward ss://method:pass@1.1.1.1:8443,udp://8.8.8.8:53
    -listen on :53 and forward all udp requests to 8.8.8.8:53 via remote ss server.

  glider -listen socks5://:1080 -listen http://:8080 -forward ss://method:pass@1.1.1.1:8443
    -listen on :1080 as socks5 server, :8080 as http proxy server, forward all requests via remote ss server.

  glider -verbose -listen -dns=:53 -dnsserver=8.8.8.8:53 -forward ss://method:pass@server:port -dnsrecord=www.example.com/1.2.3.4
    -listen on :53 as dns server, forward to 8.8.8.8:53 via ss server.
```

</details>

## Config

- [ConfigFile](config)
  - [glider.conf.example](config/glider.conf.example)
  - [office.rule.example](config/rules.d/office.rule.example)
- [Examples](config/examples)
  - [transparent proxy with dnsmasq](config/examples/8.transparent_proxy_with_dnsmasq)
  - [transparent proxy without dnsmasq](config/examples/9.transparent_proxy_without_dnsmasq)

## Service

- dhcpd: 
  - service=dhcpd,INTERFACE,START_IP,END_IP,LEASE_MINUTES[,MAC=IP,MAC=IP...]
  - e.g.:
  - service=dhcpd,eth1,192.168.1.100,192.168.1.199,720
  - service=dhcpd,eth2,192.168.2.100,192.168.2.199,720,fc:23:34:9e:25:01=192.168.2.101

## Linux Service

- systemd: [https://github.com/nadoo/glider/blob/master/systemd/](https://github.com/nadoo/glider/blob/master/systemd/)

## Customize Build

<details><summary>You can customize and build glider if you want a smaller binary (click to see details)</summary>


1. Clone the source code:
  ```bash
  git clone https://github.com/nadoo/glider && cd glider
  ```
2. Customize features:

  ```bash
  open `feature.go` & `feature_linux.go`, comment out the packages you don't need
  // _ "github.com/nadoo/glider/proxy/kcp"
  ```

3. Build it(requires **Go 1.17+** )
  ```bash
  go build -v -ldflags "-s -w"
  ```

  </details>

## Proxy & Protocol Chains
<details><summary>In glider, you can easily chain several proxy servers or protocols together (click to see details)</summary>

- Chain proxy servers:

  ```bash
  forward=http://1.1.1.1:80,socks5://2.2.2.2:1080,ss://method:pass@3.3.3.3:8443@
  ```

- Chain protocols: https proxy (http over tls)

  ```bash
  forward=tls://server.com:443,http://
  ```

- Chain protocols: vmess over ws over tls

  ```bash
  forward=tls://server.com:443,ws://,vmess://5a146038-0b56-4e95-b1dc-5c6f5a32cd98@?alterID=2
  ```

- Chain protocols and servers:

  ``` bash
  forward=socks5://1.1.1.1:1080,tls://server.com:443,vmess://5a146038-0b56-4e95-b1dc-5c6f5a32cd98@?alterID=2
  ```

- Chain protocols in listener: https proxy server

  ``` bash
  listen=tls://:443?cert=crtFilePath&key=keyFilePath,http://
  ```

- Chain protocols in listener: http over smux over websocket proxy server

  ``` bash
  listen=ws://:10000,smux://,http://
  ```

</details>

## Links

- [ipset](https://github.com/nadoo/ipset): netlink ipset package for Go.
- [conflag](https://github.com/nadoo/conflag): a drop-in replacement for Go's standard flag package with config file support.
- [ArchLinux](https://www.archlinux.org/packages/community/x86_64/glider): a great linux distribution with glider pre-built package.
- [urlencode](https://www.w3schools.com/tags/ref_urlencode.asp): you should encode special characters in scheme url. e.g., `@`->`%40`
