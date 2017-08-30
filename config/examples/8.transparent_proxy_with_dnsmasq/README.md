
## 7. Transparent Proxy with dnsmasq

#### Setup a redirect proxy and a dnstunnel with glider
glider.conf
```bash
verbose=True
listen=redir://:1081
listen=dnstun://5353=8.8.8.8:53
forward=http://forwarder1:8080,socks5://forwarder2:1080
forward=http://1.1.1.1:8080
strategy=rr
checkwebsite=www.apple.com
checkduration=30
```

#### Create a ipset manually
```bash
ipset create myset hash:ip
```

#### Config dnsmasq
```bash
server=/example1.com/127.0.0.1#5353
ipset=/example1.com/myset
server=/example2.com/127.0.0.1#5353
ipset=/example2.com/myset
server=/example3.com/127.0.0.1#5353
ipset=/example4.com/myset
```

#### Config iptables on your linux gateway
```bash
iptables -t nat -I PREROUTING -p tcp -m set --match-set myset dst -j REDIRECT --to-ports 1081
iptables -t nat -I OUTPUT -p tcp -m set --match-set myset dst -j REDIRECT --to-ports 1081
```

Now you can startup glider and dnsmasq, the whole process:
1. all dns requests for domain example1.com will be forward to glider(:5353) by dnsmasq
2. glider will forward dns requests to 8.8.8.8:53 in tcp via forwarders
3. the resolved ip address will be add to ipset "myset" by dnsmasq
4. all tcp requests to example1.com will be redirect to glider(:1081)
5. glider then forward requests to example1.com via forwarders
