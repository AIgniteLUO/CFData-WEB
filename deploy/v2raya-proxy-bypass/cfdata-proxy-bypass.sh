#!/bin/bash
# 让 CFData 容器网段绕过 v2raya 的透明代理。
#
# 背景：v2raya 会在 iptables-legacy 的 nat 表里把所有 TCP 流量 REDIRECT 到本机
# 52345 端口，再按 geosite/geoip 规则分流。它自带的豁免规则是按「目的地址」写的
# （内网段 RETURN），所以容器发往公网 Cloudflare 的流量会被送去境外代理 ——
# 表现为地区判定读到 US、ISP 判定读到境外机房、测速结果全部失真。
#
# 这里补一条按「源地址」的 RETURN，让容器流量保持直连。
#
# 必须用 iptables-legacy：宿主默认的 iptables 往往是 nf_tables 后端，与 legacy
# 是两套互不可见的内核表，用 nft 查不到 v2raya 的规则。
set -u

NETWORK="${CFDATA_NETWORK:-cfdata-web_default}"
SUBNET="$(docker network inspect "$NETWORK" --format '{{range .IPAM.Config}}{{.Subnet}}{{end}}' 2>/dev/null || true)"
# 网络暂时不存在时退回 compose 里的默认子网
SUBNET="${SUBNET:-172.27.0.0/16}"

# 幂等：已存在就不动；v2raya 重写规则把它冲掉时会重新插到最前面
if ! iptables-legacy -t nat -C PREROUTING -s "$SUBNET" -j RETURN 2>/dev/null; then
    iptables-legacy -t nat -I PREROUTING 1 -s "$SUBNET" -j RETURN
    logger -t cfdata-proxy-bypass "已插入透明代理豁免规则: $SUBNET"
fi
