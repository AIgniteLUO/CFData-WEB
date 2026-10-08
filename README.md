# CFData-Web

CFData-Web 是一个基于 Go 的 Cloudflare IP 测试与筛选工具，提供本地 Web 与 CLI 两种使用方式，支持官方 IP 段扫描、非标目标测试、测速、结果筛选、导出和 GitHub 上传。

[在线演示站](https://cfdata-demo.cce.de5.net/) 仅使用浏览器内虚拟数据，用于预览界面与交互；真实使用请下载正式版本。

![image](img/demo.png)

### 更新说明

项目功能已趋于完善，基本达到了作者预期的效果。后续更新将以维护和新功能为主，版本迭代频率会有所降低。如果你在使用过程中有新的需求或建议，欢迎提交 Issue，作者会在评估后酌情纳入后续版本。

近期主要更新：

- Docker 部署：新增 `Dockerfile`、`docker-compose.yml` 与构建推送脚本 `build-image.sh`，镜像推到私有仓库；新增 `CFDATA_` 前缀环境变量传参（命令行参数优先级更高）
- CLI 交互菜单：`-cli` 进入菜单（按配置启动 / 自定义参数启动 / 修改配置参数 / 定时任务 / 帮助），`-cli qs` 按已保存配置快速启动，`-cli -参数…` 直接执行；启动时输出 Banner 与「调试等级」
- 参数设置向导：分模块逐项设置，提示格式与可选值，可保存为配置文件
- 定时任务：仅 Linux systemd，按天/周/月执行，开机自启，失败自动回滚清理，unit 与日志在程序目录
- 官方/非标的扫描并发、延迟阈值、测速网址在命令行与配置文件中完全独立，互不影响
- 配置文件自动迁移：键名无变化时静默按新模板迁移；存在旧键名时交互提示（仅迁移同名值），原文件备份为 `.bak`，非交互环境自动跳过
- 启动输出统一为简洁的「当前配置」单块，版本行下方显示调试等级

## 功能

- 官方优选：扫描 Cloudflare IPv4/IPv6，按数据中心继续详细延迟测试。
- 非标优选：上传本地 txt/csv 或填写网络 URL，测试自定义 IP/域名与端口。
- 测速：支持单点测速、批量测速、非标并发测速和测速阈值筛选。
- 导出：支持 CSV/TXT、自定义字段、IP 类型筛选、合格结果筛选。
- 上传：支持将导出结果上传到 GitHub。
- CLI：交互菜单、参数设置向导、定时任务、配置自动迁移。
- APK：支持 Android WebView 壳运行内置后端。
- Docker：Docker Compose 部署，镜像可推送到私有仓库；支持用环境变量传参。

## 快速开始

从 [Releases](https://github.com/PoemMisty/CFData-WEB/releases/latest) 下载对应平台程序后运行。

默认启动 Web 模式：

```text
服务启动于 http://localhost:13335
当前测速网址: auto
```

浏览器打开终端中的地址即可使用。

CLI 模式（进入交互菜单）：

```bash
./cfdata-linux-amd64 -cli
```

菜单提供：按配置启动（菜单 1，等价 `-cli qs`）、自定义参数启动（向导）、修改配置参数、定时任务、帮助。

首次使用 CLI 配置文件时会生成模板并退出，编辑配置后重新运行即可；也可通过菜单「修改配置参数」逐项设置。升级后启动时若配置键名有变化会提示迁移：同名值保留，原文件备份为 `.bak`，非交互环境自动跳过。

简单示例：

```bash
# 交互菜单（不带其他参数时默认进入菜单）
./cfdata-linux-amd64 -cli

# 按已保存配置快速启动（qs 大小写不敏感）
./cfdata-linux-amd64 -cli qs

# 官方模式直接执行：扫描 IPv4，测试 443 端口，测速地址自动选择
./cfdata-linux-amd64 -cli -mode official -offiptype 4 -offport 443 -offurl auto

# 非标模式直接执行：读取本地文件，开启 TLS 和 5 个测速线程
./cfdata-linux-amd64 -cli -mode nsb -nsbfile ip.txt -nsbtls=true -nsbspeedtest 5 -nsburl auto
```

## Docker 部署

适合在服务器上长期运行。镜像位于私有仓库 `registry.vxk8s.com/cfdata-web/cfdata-web`。

### 服务器部署

```bash
cp .env.example .env     # 按需修改，公网部署务必设置认证
docker compose pull
docker compose up -d
docker compose logs -f
```

宿主机端口由 `.env` 中的 `CFDATA_HOST_PORT` 决定（默认 13335），容器内固定监听 13335。

### 环境变量

优先级：命令行参数 > 环境变量 > 默认值。变量统一使用 `CFDATA_` 前缀，避免与容器内其他工具的环境变量撞名。

```text
CFDATA_PORT        容器内监听端口，默认 13335
                   compose 部署不必改这项，宿主端口请用 .env 的 CFDATA_HOST_PORT
CFDATA_HOST        监听地址，默认空（监听全部地址）
CFDATA_USER        Web 认证用户名，默认空（不启用认证）
CFDATA_PASSWORD    Web 认证密码，需与 CFDATA_USER 同时设置
CFDATA_SESSION     登录会话有效期（分钟），默认 720
CFDATA_DEBUG       调试等级：error 或 all，默认关闭
CFDATA_SKIPGEO     跳过地区/代理环境验证，默认 false
```

空值等同于未设置；非法值只打印告警并忽略，不会导致启动失败。`CFDATA_DEBUG` 只接受能明确识别的值（`error`、`all`、`true`/`false` 等），写错不会静默把调试日志打开。

环境变量能让密码不出现在 `docker ps` 输出和进程命令行里，但 `docker inspect` 与 `.env` 文件仍可读到，属于配置传递而非密钥托管。有更高保密要求时请改用 Docker secrets。

### 数据持久化

`/app/data` 是程序的工作目录，地址库 `ips-v4.txt`/`ips-v6.txt`、地区库 `locations.json`、ASN 库 `GeoLite2-ASN.mmdb`、调试日志 `cfdata-debug.log`，以及导出与上传用的结果文件都写在这里，由具名卷 `cfdata-data` 持久化，重建容器无需重新下载。

取出结果文件：

```bash
docker compose cp cfdata:/app/data/. ./data        # 整个目录拷出来
docker compose exec cfdata ls -la /app/data        # 或者先看看有什么
```

若改用 bind mount（如 `./data:/app/data`），宿主目录默认归 root，容器里的 uid 10001 写不进去，表现为每次重建容器都要重新下载缓存；需要先 `mkdir -p data && sudo chown 10001:10001 data`。

`cfdata-config.json` 落在 `/app`（可执行文件所在目录）。Web 模式下它只是首次启动生成的默认模板、界面只读，重建容器会重新生成一份等价模板（其中 `_config_version` 跟随版本号），因此不必单独挂卷。

### 自行构建与推送

```bash
docker login registry.vxk8s.com
./build-image.sh              # 构建 linux/amd64，打「版本号」与 latest 两个 tag 并推送
./build-image.sh --local      # 只构建并载入本机，不推送（本地验证用）
VERSION=v1.2.3 ./build-image.sh
```

版本号默认取 `git describe --tags --always`，工作区有改动时追加 `-dirty`。国内构建可换 Go 模块源：`GOPROXY=https://goproxy.cn,direct ./build-image.sh`。

推送会同时覆盖 `:latest`，脚本默认拦截非 `linux/amd64` 的推送（服务器拉取的正是 amd64 的 `latest`，用别的架构覆盖会让它拿到跑不起来的镜像），确实需要时设置 `ALLOW_FOREIGN_PLATFORM=1`。

### 公网部署建议

镜像默认不启用认证且监听全部地址，直接暴露到公网等于把扫描与测速能力开放给任何人。建议：

1. 设置 `CFDATA_USER` 与 `CFDATA_PASSWORD`。
2. 只在本机暴露端口，前面挂 Caddy/nginx 做 TLS 终止 —— `docker-compose.yml` 的 `ports` 段已给出注释掉的 `127.0.0.1:` 写法。
3. 注意：程序自身不支持 TLS，判定 cookie 是否加 `Secure` 依据的是 `r.TLS`，反向代理终止 TLS 时它永远为 false。也就是说即使套了 HTTPS，会话 cookie 也不会带 `Secure` 标记，请确保从代理到容器的这一段不经过不可信网络（同机部署即可满足）。

### 注意事项

- **IPv6**：容器内测试 Cloudflare IPv6 地址段需要宿主 Docker daemon 启用 IPv6（`/etc/docker/daemon.json` 配置 `"ipv6": true` 与 `"fixed-cidr-v6"`），否则 IPv6 结果会全部失败。
- **测速精度**：默认 bridge 网络的 NAT 会带来额外开销。需要更准确的结果，可在 `docker-compose.yml` 中启用 `network_mode: host`（启用后 `ports` 段失效，容器直接占用宿主 13335 端口）。
- **密码含 `$`**：写在 `.env` 里必须写成 `$$`，否则会被 compose 当成变量插值截断，症状是「密码明明填对了却登不进去」。
- **健康检查**：由镜像内置，探测不经过认证的 `/favicon.png`，并跟随 `CFDATA_PORT`，compose 中无需重复声明。
- 容器以非 root（uid 10001）运行，二进制归 root 所有、内容不可被应用改写。

## Web 使用

界面顶部「扫描方式」选择器支持 TCPing（默认）和 HTTPing。不同扫描模式的延迟数据不可互相比较，仅同模式内的对比才有意义。

### 扫描方式说明

- **TCPing**：测量 TCP 握手延迟，基准值。
- **HTTPing**：测量 HTTP TTFB（Time To First Byte），延迟比 TCPing 高属正常现象。延迟阈值和渲染颜色已按倍率自动缩放，倍率仅为延迟等级参考值，非精确换算：
  - 无 TLS（HTTP 端口）：×1.3
  - 有 TLS（HTTPS 端口）：×4.0

### 官方优选

1. 选择 IPv4 或 IPv6。
2. 设置测试端口、扫描并发、延迟阈值。
3. 点击“开始扫描与测试”。
4. 扫描完成后选择数据中心继续详细测试。
5. 在详细测试结果中可单点测速或批量测速。

### 非标优选

1. 切换到“非标优选”。
2. 上传 txt/csv，或填写网络 URL（二选一）。
3. 设置备用端口、并发、TLS、结果上限、测速线程、测速阈值等参数。
4. 点击“开始扫描与测试”。
5. 在结果表格查看、筛选、导出或上传。

非标输入推荐格式：

```text
1.2.3.4 443
5.6.7.8 8443
2606:4700::1111 443
1.1.1.1
```

未提供端口时会使用备用端口；备用端口默认随 TLS 模式自动选择，关闭 TLS 为 80，开启 TLS 为 443。

## 测速地址

默认测速地址为 `auto`，表示由后端自动选择内置测速源。

Web 下拉项：

- 自动选择
- Cloudflare
- CM提供
- 移动专属
- 手动输入

CLI 可通过 `-offurl`/`-nsburl` 指定：

```bash
./cfdata-linux-amd64 -cli -offurl auto
./cfdata-linux-amd64 -cli -offurl speed.cloudflare.com/__down?bytes=99999999
./cfdata-linux-amd64 -cli -offurl https://example.com/file.bin
```

说明：测速只读取响应字节流计算速度，不会把测速文件保存到本地。

## 常用参数

```text
-cli              启用 CLI 模式
-mode             official 或 nsb
-scanmode         扫描方式：tcping（默认，TCP 握手延迟）或 httping（HTTP TTFB，延迟比 tcping 高属正常，不同模式数据不可对比）
-offthreads       官方扫描并发数
-nsbthreads       非标扫描并发数
-offport          官方测试/测速端口
-offdelay         官方延迟阈值，单位毫秒
-nsbdelay         非标延迟阈值，单位毫秒；0=不筛延迟
-offurl           官方测速下载地址，默认 auto
-nsburl           非标测速下载地址，默认 auto
-dns              自定义 DNS 服务器
-debug            调试日志等级：false、error、all
-skipgeo          跳过地区/代理环境验证（启动时不再提示代理警告）
-out              输出文件名（官方/非标通用），默认 cfdata-results
```

非标常用参数：

```text
-nsbfile          本地输入文件
-nsbsourceurl     网络输入 URL
-nsbfallbackport  非标输入缺省端口；不传时随 TLS 自动使用 443/80
-nsbtls           非标是否启用 TLS
-nsbspeedtest     非标测速线程数，0 表示不测速。多 IP 并发影响实际速度，需要准确应设为 1
-nsbresultlimit   非标延迟测试结果上限
-nsbspeedmin      非标测速合格阈值，单位 MB/s
-nsbspeedlimit    非标测速合格结果上限
```

完整参数可运行：

```bash
./cfdata-linux-amd64 -h
```

## 本地缓存

Web 右上角设置菜单提供“恢复全部默认配置”，会清理本地缓存文件，例如 `ips-v4.txt`、`ips-v6.txt`、`locations.json`、ASN 数据库等。任务运行中不会直接清理，避免影响测试。

## 免责声明

本程序仅限用于学习与研究目的。请在下载后24小时内自行删除。使用本程序时，应自行遵守所在地区的法律法规。作者不对使用本程序所产生的任何后果承担责任。下载或使用本程序即视为已阅读、理解并同意上述声明。

## 致谢

- TG 频道：[CF中转IP](https://t.me/CF_NAT)
- GitHub：[Kwisma/iptest](https://github.com/Kwisma/iptest)

## License

Copyright (C) 2026 PoemMisty

This project is licensed under the GNU General Public License v3.0 or later.
See the LICENSE file for details.
