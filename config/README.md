
## Config File 
Command:
```bash
glider -config glider.conf
```
Config file, **just use the command line flag name as the key name**:
```bash
### glider config file

# verbose mode, print logs
verbose

# listen on 8443, serve as http/socks5 proxy on the same port.
listen=:8443

# listen on udp port 5353, forward dns requests via tcp protocol
listen=dnstun://:5353=8.8.8.8:53

# upstream forward proxy
forward=socks5://192.168.1.10:1080

# upstream forward proxy
forward=ss://method:pass@1.1.1.1:8443

# upstream forward proxy (forward chain)
forward=http://1.1.1.1:8080,socks5://2.2.2.2:1080

# multiple upstream proxies forwad strategy
strategy=rr

# Used to connect via forwarders, if the host is unreachable, the forwarder
# will be set to disabled.
# MUST be a HTTP website server address, format: HOST[:PORT]. HTTPS NOT SUPPORTED.
checkwebsite=www.apple.com

# check duration
checkduration=30


# Setup a dns forwarding server
dns://53
# global remote dns server (you can specify different dns server in rule file)
dnsserver=8.8.8.8:53

# Create and manage ipset on linux based on destinations in rule files
#   - add ip/cidrs in rule files on startup
#   - add resolved ips for domains in rule files by dns forwarder server 
# Usually used in transparent proxy mode on linux
ipset=glider

# RULE FILES
rules-dir=rules.d
#rulefile=office.rule
#rulefile=home.rule
```
See:
- [glider.conf.example](config/glider.conf.example)
- [examples](config/examples)

## Rule File
Rule file, **same as the config file but specify forwarders based on destinations**:
```bash
# YOU CAN USE ALL KEYS IN THE GLOBAL CONFIG FILE EXCEPT "listen", "rulefile"
forward=socks5://192.168.1.10:1080
forward=ss://method:pass@1.1.1.1:8443
forward=http://192.168.2.1:8080,socks5://192.168.2.2:1080
strategy=rr
checkwebsite=www.apple.com
checkduration=30

# DNS SERVER for domains in this rule file
dnsserver=208.67.222.222:53

# YOU CAN SPECIFY DESTINATIONS TO USE THE ABOVE FORWARDERS
# matches abc.com and *.abc.com
domain=abc.com

# matches 1.1.1.1
ip=1.1.1.1

# matches 192.168.100.0/24
cidr=192.168.100.0/24

# we can include a list file with only destinations settings
include=office.list.example

```
See:
- [office.rule.example](rules.d/office.rule.example)
- [examples](examples)
