
# Glider Configuration Examples

## Simple Proxy Service
Just listen on 8443 as HTTP/SOCKS5 proxy on the same port, forward all requests directly.

```      
   Clients --> Listener --> Internet
```

- [simple_proxy_service](simple_proxy_service)

## One remote upstream proxy

```
   Clients --> Listener --> Forwarder -->  Internet
```

- [one_forwarder](one_forwarder)

## One remote upstream PROXY CHAIN

```
   Clients -->  Listener --> Forwarder1 --> Forwarder2 -->  Internet
```

- [forward_chain](forward_chain)

## Multiple upstream proxies

```
                            |Forwarder ----------------->|         
   Clients --> Listener --> |                            | Internet
                            |Forwarder --> Forwarder->...| 
```

- [multiple_forwarders](multiple_forwarders)


## With Rule File: Default Direct, Rule file use forwarder

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

- [rule_default_direct](rule_default_direct)


## With Rule File: Default use forwarder, rule file use direct

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

- [rule_default_forwarder](rule_default_forwarder)


## With Rule File: multiple rule files

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

- [rule_multiple_rule_files](rule_multiple_rule_files)