
## 9. Transparent Proxy without dnsmasq

PC Client -> Gateway with glider running(linux box) -> Upstream Forwarders -> Internet

#### In this mode, glider will act as the following roles:
1. A transparent proxy server
2. A dns forwarding server
3. A ipset manager

so you don't need any dns server in your network.

#### Create a ipset manually
```bash
ipset create glider hash:net
```

#### Glider Configuration
##### glider.conf
```bash
verbose=True

# as a redir proxy
listen=redir://:1081

# as a dns forwarding server
dns=:53
dnsserver=8.8.8.8:53
dnsserver=8.8.4.4:53

# specify rule files
rules-dir=rules.d
```

##### office.rule
```bash
# add your forwarders
forward=http://forwarder1:8080,socks5://forwarder2:1080
forward=http://1.1.1.1:8080
strategy=rr
check=http://www.msftconnecttest.com/connecttest.txt#expect=200

# specify a different dns server(if need)
dnsserver=208.67.222.222:53

# as a ipset manager
ipset=glider

# specify destinations
include=office.list

domain=example1.com
domain=example2.com
# matches ip
ip=1.1.1.1
ip=2.2.2.2
# matches a ip net
cidr=192.168.100.0/24
cidr=172.16.100.0/24
```

##### office.list
```bash
# destinations list
domain=mycompany.com
domain=mycompany1.com
ip=4.4.4.4
ip=5.5.5.5
cidr=172.16.101.0/24
cidr=172.16.102.0/24
```

#### Configure iptables on your linux gateway
```bash
iptables -t nat -I PREROUTING -p tcp -m set --match-set glider dst -j REDIRECT --to-ports 1081
iptables -t nat -I OUTPUT -p tcp -m set --match-set glider dst -j REDIRECT --to-ports 1081
```

#### Server DNS settings
Set server's nameserver to glider:
```bash
echo nameserver 127.0.0.1 > /etc/resolv.conf
```

#### Client settings
Use the linux server's ip as your gateway.
Use the linux server's ip as your dns server.

#### When client requesting to access http://example1.com (in office.rule), the whole process:
DNS Resolving: 
1. client sends a udp dns request to linux server, and glider will receive the request(as it listens on the default dns port :53)
2. upstream dns server choice: glider will lookup it's rule config and find out the dns server to use for this domain(matched "example1.com" in office.rule, so 208.67.222.222:53 will be chosen)
3. glider uses the forwarder in office.rule to ask 208.67.222.222:53 for the resolve answers(dns over proxy).
4. glider updates it's office rule config, adds the resolved ip address to it.
5. glider adds the resolved ip into ipset "glider", and returns the dns answer to client.

Destination Accessing:
1. client sends http request to the resolved ip of example1.com.
2. linux gateway server will get the request.
3. iptables matches the ip in ipset "glider" and redirect this request to :1081(glider)
4. glider finds the ip in office rule, and then choose a forwarder in office.rule to complete the request.
