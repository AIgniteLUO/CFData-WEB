# v2raya 透明代理豁免

## 症状

CFData 在 Docker 中启动后，Web 界面提示「代理检测警告：当前网络标签 US」，但宿主机并没有配置代理环境变量：

- 容器日志里**没有** `出网代理: ...` 这一行（说明容器内确实没有 `HTTP_PROXY`/`HTTPS_PROXY`）
- `docker compose exec cfdata env | grep -i proxy` 为空
- 但容器访问 `https://www.cloudflare.com/cdn-cgi/trace` 的出口仍是境外 IP

## 原因

宿主机上的 v2raya 开启了透明代理，规则写在 **iptables-legacy** 的 nat 表里：

```
-A PREROUTING -p tcp -j TP_PRE
-A OUTPUT     -p tcp -j TP_OUT
-A TP_RULE -d 172.27.0.0/16 -j RETURN        # 这些豁免匹配的是「目的地址」
-A TP_RULE -d 192.168.99.0/24 -j RETURN
...
-A TP_RULE -p tcp -j REDIRECT --to-ports 52345   # 兜底：所有 TCP 全量重定向
```

所有 TCP 流量（本机 OUTPUT 与转发 PREROUTING）都会被重定向到本机 v2ray，再由它按 geosite/geoip 规则分流。自带的 `RETURN` 规则按**目的地址**豁免内网段，因此**容器发往公网 Cloudflare 的流量照样被送去境外代理**。

> 排查时容易走弯路：宿主默认的 `iptables` 是 `nf_tables` 后端，与 `iptables-legacy` 是两套互不可见的内核表。
> `nft list ruleset` 和 `iptables -t nat -L` 都看不到上面这些规则，必须用 `iptables-legacy-save`。
> 判断方法：`iptables-save` 会打印 `Warning: iptables-legacy tables present`。

## 解决

在 `iptables-legacy` 的 nat `PREROUTING` 链最前面插一条按**源地址**的 `RETURN`：

```bash
iptables-legacy -t nat -I PREROUTING 1 -s <CFData 容器子网> -j RETURN
```

只影响 CFData 容器的出网路径，宿主机自身与其他容器的代理行为不变。

## 安装

```bash
install -m 0755 cfdata-proxy-bypass.sh /usr/local/bin/
install -m 0644 cfdata-proxy-bypass.service /etc/systemd/system/
install -m 0644 cfdata-proxy-bypass.timer   /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now cfdata-proxy-bypass.timer
systemctl start cfdata-proxy-bypass.service
```

脚本是幂等的：规则已存在就不动，被 v2raya 重写冲掉后会重新插回链首。定时器在开机 20 秒后触发，之后每 60 秒校正一次，用来兜住 v2raya 重连时重建规则的情况。

容器子网默认从 `cfdata-web_default` 网络动态读取；网络暂时不存在时回退到 `172.27.0.0/16`。可用环境变量 `CFDATA_NETWORK` 指定其他网络名。

## 验证

```bash
# 规则应在 PREROUTING 第 1 位
iptables-legacy -t nat -L PREROUTING -n --line-numbers

# 容器出口应为本地网络的真实地区，而不是代理出口
docker compose exec cfdata wget -qO- https://www.cloudflare.com/cdn-cgi/trace | grep -E '^(ip|loc)='
```

## 卸载

```bash
systemctl disable --now cfdata-proxy-bypass.timer
rm -f /etc/systemd/system/cfdata-proxy-bypass.{service,timer} /usr/local/bin/cfdata-proxy-bypass.sh
systemctl daemon-reload
iptables-legacy -t nat -D PREROUTING -s 172.27.0.0/16 -j RETURN
```
