
# Glider Configuration Examples

## 1. Simple Proxy Service
Just listen on 8443 as HTTP/SOCKS5 proxy on the same port, forward all requests directly.

```      
   Clients --> Listener --> Internet
```

- [simple_proxy_service](1.simple_proxy_service)

## 2. One remote upstream proxy

```
   Clients --> Listener --> Forwarder -->  Internet
```

- [one_forwarder](2.one_forwarder)

## 3. One remote upstream PROXY CHAIN

```
   Clients -->  Listener --> Forwarder1 --> Forwarder2 -->  Internet
```

- [forward_chain](3.forward_chain)

## 4. Multiple upstream proxies

```
                            |Forwarder ----------------->|         
   Clients --> Listener --> |                            | Internet
                            |Forwarder --> Forwarder->...| 
```

- [multiple_forwarders](4.multiple_forwarders)


## 5. With Rule File: Default Direct, Rule file use forwarder

Default:
```
   Clients --> Listener --> Internet
```
Destinations specified in rule file:
```
                             |Forwarder ----------------->|         
   Clients -->  Listener --> |                            | Internet
                             |Forwarder --> Forwarder->...| 
```

- [rule_default_direct](5.rule_default_direct)


## 6. With Rule File: Default use forwarder, rule file use direct

Default:
```
                             |Forwarder ----------------->|         
   Clients -->  Listener --> |                            | Internet
                             |Forwarder --> Forwarder->...| 
```

Destinations specified in rule file:
```
   Clients --> Listener --> Internet
```

- [rule_default_forwarder](6.rule_default_forwarder)


## 7. With Rule File: multiple rule files

Default:
```
   Clients --> Listener --> Internet
```
Destinations specified in rule file1:
```
                             |Forwarder1 ----------------->|         
   Clients -->  Listener --> |                            | Internet
                             |Forwarder2 --> Forwarder3->...| 
```
Destinations specified in rule file2:
```
                             |Forwarder4 ----------------->|         
   Clients -->  Listener --> |                            | Internet
                             |Forwarder5 --> Forwarder6->...| 
```

- [rule_multiple_rule_files](7.rule_multiple_rule_files)

## 8. Transparent Proxy with Dnsmasq
- [transparent_proxy_with_dnsmasq](8.transparent_proxy_with_dnsmasq)

## 9. Transparent Proxy without Dnsmasq
- [transparent_proxy_without_dnsmasq](9.transparent_proxy_without_dnsmasq)