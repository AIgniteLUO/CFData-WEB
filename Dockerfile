# syntax=docker/dockerfile:1

# ---------- 构建阶段 ----------
# 用 $BUILDPLATFORM 让构建阶段跑在原生架构上（Apple Silicon 上就是原生 arm64），
# 只把产物交叉编译成 $TARGETARCH，省掉 QEMU 模拟带来的几十倍耗时。
# 钉住补丁版本：go.mod 要求 go 1.25.4，浮动 tag 一旦命中本地缓存的旧镜像，
# 会因为 GOTOOLCHAIN=local 直接构建失败而不是自动补工具链。
FROM --platform=$BUILDPLATFORM golang:1.25.14-alpine AS builder

ARG TARGETARCH
ARG VERSION=dev
# 国内构建可覆盖：--build-arg GOPROXY=https://goproxy.cn,direct
ARG GOPROXY=https://proxy.golang.org,direct

ENV CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} GOPROXY=${GOPROXY}

WORKDIR /src

# 单独先拉依赖，源码变动时这一层仍可命中缓存
COPY combined_refactor/go.mod combined_refactor/go.sum ./
RUN go mod download

COPY combined_refactor/ ./

RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags "-s -w -X main.appVersion=${VERSION}" -o /out/cfdata .

# ---------- 运行阶段 ----------
FROM alpine:3.21

ARG VERSION=dev
ARG REVISION=

LABEL org.opencontainers.image.title="CFData-WEB" \
      org.opencontainers.image.description="Cloudflare IP 测试与筛选工具，提供 Web 界面" \
      org.opencontainers.image.source="https://github.com/PoemMisty/CFData-WEB" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.licenses="GPL-3.0-or-later"

# ca.go 的 rootCAPool() 优先使用系统信任库、内嵌证书包仅作兜底，
# 装上 ca-certificates 才走主路径。不装 tzdata：程序未用 time.LoadLocation，日志固定 UTC 即正确行为。
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 -h /app cfdata

COPY --from=builder /out/cfdata /app/cfdata

# 程序有两处写盘点，是它本身的既有行为：
#   - 工作目录：地址库 / 地区库 / ASN 库 / 调试日志（storage.go、asn.go、debug.go 都走相对路径）
#   - 可执行文件所在目录：cfdata-config.json（cli.go 用 os.Executable() 取目录）
# 所以 /app/data 作为工作目录交给卷挂载，/app 本身也需可写（Web 界面要读这份配置，
# 首次启动要能写出模板，故不能做成只读）。二进制保持 root 所有，应用用户改不了它的内容。
RUN mkdir -p /app/data \
    && chown cfdata:cfdata /app /app/data

USER cfdata
WORKDIR /app/data
EXPOSE 13335

# ENTRYPOINT 必须写绝对路径：server.go 用 filepath.Dir(os.Args[0]) 定位配置文件，
# 一旦退化成走 PATH 查找的裸文件名，目录会被解析成当前目录，配置文件就跑到 /app/data 去了。
#
# 这里刻意不设 CMD 指定 -host：listenHost 为空时 main.go 生成 ":13335"，
# 在 Linux 上是双栈监听，比写死 "0.0.0.0:13335"（纯 IPv4）更好；
# 且一旦写成命令行参数，就会永久压过 CFDATA_HOST，让那个环境变量形同虚设。
# 要改监听地址请用 CFDATA_HOST，或在 compose 里覆盖 command。
ENTRYPOINT ["/app/cfdata"]

# HEALTHCHECK 只认 CMD / NONE 两种形式（CMD-SHELL 是 compose 的写法，这里会直接解析失败），
# 写 CMD 的 shell 形式即可，构建后会被存成 CMD-SHELL 并由 /bin/sh 执行。
# 这里必须用单个 $：HEALTHCHECK 不受 Dockerfile 构建期变量替换影响，写 $$ 不会被还原成 $，
# 反而会在运行期被 shell 解释成自身 PID，URL 直接失效（实测踩过）。
# /favicon.png 不经过 requireAuth，开启认证后依然能探测（main.go 中它注册在 requireAuth 之外）。
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -q --spider "http://127.0.0.1:${CFDATA_PORT:-13335}/favicon.png" || exit 1
