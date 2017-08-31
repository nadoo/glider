
## 9. Transparent Proxy without dnsmasq

In this mode, glider will act as the following roles:
1. A transparent proxy server
2. A dns forwarding server
3. A ipset manager
so you don't need any dns server in your network.

#### Glider Configuration
##### glider.conf
```bash
verbose=True
# as a redir proxy
listen=redir://:1081
# as a dns forwarding server
dns=:53
dnsserver=8.8.8.8:53
# as a ipset manager
ipset=glider
# specify rule files
rules-dir=rules.d
```

##### office.rule
```bash
# add your forwarders
forward=http://forwarder1:8080,socks5://forwarder2:1080
forward=http://1.1.1.1:8080
strategy=rr
checkwebsite=www.apple.com
checkduration=30
# specify a different dns server(if need)
dnsserver=208.67.222.222:53

# specify destinations
#include=office.list.example
domain=example1.com
domain=example2.com
# matches ip
ip=1.1.1.1
ip=2.2.2.2
# matches a ip net
cidr=192.168.100.0/24
cidr=172.16.100.0/24
```

#### Config iptables on your linux gateway
```bash
iptables -t nat -I PREROUTING -p tcp -m set --match-set glider dst -j REDIRECT --to-ports 1081
iptables -t nat -I OUTPUT -p tcp -m set --match-set glider dst -j REDIRECT --to-ports 1081
```

Now you can startup glider and dnsmasq, the whole process:
1. 
1. all dns requests for domain example1.com will be forward to glider(:5353) by dnsmasq
2. glider will forward dns requests to 8.8.8.8:53 in tcp via forwarders
3. the resolved ip address will be add to ipset "myset" by dnsmasq
4. all tcp requests to example1.com will be redirect to glider(:1081)
5. glider then forward requests to example1.com via forwarders
