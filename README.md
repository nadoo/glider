# [glider](https://github.com/nadoo/glider)

[![Go Report Card](https://goreportcard.com/badge/github.com/nadoo/glider?style=flat-square)](https://goreportcard.com/report/github.com/nadoo/glider)
[![GitHub release](https://img.shields.io/github/v/release/nadoo/glider.svg?style=flat-square&include_prereleases)](https://github.com/nadoo/glider/releases)
[![Actions Status](https://img.shields.io/github/workflow/status/nadoo/glider/Build?style=flat-square)](https://github.com/nadoo/glider/actions)

glider is a forward proxy with multiple protocols support, and also a dns forwarding server with ipset management features(like dnsmasq).

we can set up local listeners as proxy servers, and forward requests to internet via forwarders.

```bash
                |Forwarder ----------------->|
   Listener --> |                            | Internet
                |Forwarder --> Forwarder->...|
```

## Features

Listen (local proxy server):

- Socks5 proxy(tcp&udp)
- Http proxy(tcp)
- SS proxy(tcp&udp)
- Linux transparent proxy(iptables redirect)
- TCP tunnel
- UDP tunnel
- UDP over TCP tunnel
- TLS, use it together with above proxy protocols(tcp)
- Unix domain socket, use it together with above proxy protocols(tcp)
- KCP protocol, use it together with above proxy protocols(tcp)

Forward (local proxy client/upstream proxy server):

- Socks5 proxy(tcp&udp)
- Http proxy(tcp)
- SS proxy(tcp&udp&uot)
- SSR proxy(tcp)
- VMess proxy(tcp)
- Trojan proxy(tcp)
- TLS, use it together with above proxy protocols(tcp)
- Websocket, use it together with above proxy protocols(tcp)
- Unix domain socket, use it together with above proxy protocols(tcp)
- KCP protocol, use it together with above proxy protocols(tcp)
- Simple-Obfs, use it together with above proxy protocols(tcp)

DNS Forwarding Server (udp2tcp):

- DNS Over Proxy
- Listen on UDP and forward dns requests to remote dns server in TCP via forwarders
- Specify different upstream dns server based on destinations(in rule file)
- Tunnel mode: forward to a fixed upstream dns server
- Add resolved IPs to proxy rules
- Add resolved IPs to ipset
- DNS cache
- Custom dns record

IPSet Management (Linux kernel version >= 2.6.32):

- Add ip/cidrs from rule files on startup
- Add resolved ips for domains from rule files by dns forwarding server

General:

- Http and socks5 on the same port
- Forwarder chain
- RR/HA/LHA/DH strategy for multiple forwarders
- Periodical proxy checking
- Rule proxy based on destinations: [Config Examples](config/examples)
- Send requests from specific ip/interface

TODO:

- [ ] IPv6 support in ipset manager
- [ ] Transparent UDP proxy (iptables tproxy)
- [ ] Performance tuning
- [ ] TUN/TAP device support
- [ ] SSH tunnel support (maybe)

## Install

Binary:

- [https://github.com/nadoo/glider/releases](https://github.com/nadoo/glider/releases)

Go Get (requires **Go 1.14+** ):

```bash
go get -u github.com/nadoo/glider
```

ArchLinux:

```bash
sudo pacman -S glider
```

## Run

help:
```bash
glider -h
```

run:
```bash
glider -verbose -listen :8443 -forward SCHEME://HOST:PORT
```
```bash
glider -config CONFIGPATH
```
```bash
glider -config CONFIGPATH -listen :8080 -verbose
```

## Config

- [ConfigFile](config)
  - [glider.conf.example](config/glider.conf.example)
  - [office.rule.example](config/rules.d/office.rule.example)
- [Examples](config/examples)
  - [transparent proxy with dnsmasq](config/examples/8.transparent_proxy_with_dnsmasq)
  - [transparent proxy without dnsmasq](config/examples/9.transparent_proxy_without_dnsmasq)

## Proxy & Protocol Chain
In glider, you can easily chain several proxy servers or protocols together, e.g:

- Chain proxy servers:

```bash
forward=http://1.1.1.1:80,socks5://2.2.2.2:1080,ss://method:pass@3.3.3.3:8443@
```

- Chain protocols: https proxy (http over tls)

```bash
forward=tls://1.1.1.1:443,http://
```

- Chain protocols: vmess over ws over tls

```bash
forward=tls://1.1.1.1:443,ws://,vmess://5a146038-0b56-4e95-b1dc-5c6f5a32cd98@?alterID=2
```

- Chain protocols and servers:

``` bash
forward=socks5://1.1.1.1:1080,tls://2.2.2.2:443,ws://,vmess://5a146038-0b56-4e95-b1dc-5c6f5a32cd98@?alterID=2
```

- Chain protocols in listener: https proxy server

``` bash
listen=tls://:443?cert=crtFilePath&key=keyFilePath,http://
```


## Service

- systemd: [https://github.com/nadoo/glider/blob/master/systemd/](https://github.com/nadoo/glider/blob/master/systemd/)

## Links

- [conflag](https://github.com/nadoo/conflag): command line and config file parse support
- [ArchLinux](https://www.archlinux.org/packages/community/x86_64/glider): a great linux distribution with glider pre-built package
