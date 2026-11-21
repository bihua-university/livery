package profile

const defaultMihomoStarterConfig = `mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
external-controller: 127.0.0.1:9090
dns:
  enable: true
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  fake-ip-filter:
    - "+.lan"
    - "+.local"
    - "+.ts.net"
    - "dns.msftnsci.com"
    - "www.msftnsci.com"
    - "www.msftconnecttest.com"
  proxy-server-nameserver:
    - 223.5.5.5
    - 119.29.29.29
  nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query
    - 223.5.5.5
  fallback:
    - https://dns.cloudflare.com/dns-query
    - https://dns.google/dns-query
  fallback-filter:
    geoip: true
    geoip-code: CN
    ipcidr:
      - 240.0.0.0/4
  nameserver-policy:
    "+.ts.net": system
proxies: []
proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - DIRECT
rules:
  - DOMAIN-SUFFIX,ts.net,DIRECT
  - IP-CIDR,100.64.0.0/10,DIRECT,no-resolve
  - IP-CIDR6,fd7a:115c:a1e0::/48,DIRECT,no-resolve
  - IP-CIDR,127.0.0.0/8,DIRECT,no-resolve
  - IP-CIDR,172.16.0.0/12,DIRECT,no-resolve
  - IP-CIDR,192.168.0.0/16,DIRECT,no-resolve
  - MATCH,Proxy
`

const defaultStarterConfig = `{
  "log": {
    "level": "info",
    "timestamp": true
  },
  "dns": {
    "servers": [
      { "type": "local", "tag": "dns-local" },
      { "type": "udp", "tag": "dns-direct", "server": "223.5.5.5" },
      { "type": "udp", "tag": "dns-remote", "server": "1.1.1.1", "detour": "proxy" }
    ],
    "rules": [
      { "domain_suffix": ["ts.net"], "server": "dns-local" }
    ],
    "final": "dns-remote"
  },
  "inbounds": [
    {
      "type": "mixed",
      "tag": "mixed-in",
      "listen": "127.0.0.1",
      "listen_port": 7890
    }
  ],
  "outbounds": [
    {
      "type": "selector",
      "tag": "proxy",
      "outbounds": ["auto", "direct"]
    },
    {
      "type": "urltest",
      "tag": "auto",
      "outbounds": ["direct"],
      "url": "http://www.gstatic.com/generate_204",
      "interval": "3m"
    },
    {
      "type": "direct",
      "tag": "direct"
    }
  ],
  "route": {
    "default_domain_resolver": "dns-direct",
    "rules": [
      { "protocol": "dns", "action": "hijack-dns" },
      { "domain_suffix": ["ts.net"], "outbound": "direct" },
      { "ip_cidr": ["100.64.0.0/10", "fd7a:115c:a1e0::/48"], "outbound": "direct" },
      { "ip_is_private": true, "outbound": "direct" }
    ],
    "final": "proxy",
    "auto_detect_interface": true
  }
}
`
