#!/usr/bin/env bash
# 构建 CFData-WEB 镜像并推送到私有仓库 registry.vxk8s.com
#
#   ./build-image.sh                 # 构建 linux/amd64，打「版本号」与 latest 两个 tag 并推送
#   ./build-image.sh --local         # 只构建并载入本机（供 docker compose 本地验证），不推送
#   VERSION=v1.2.3 ./build-image.sh  # 手动指定版本号
#
# 推送前请先登录私有仓库：docker login registry.vxk8s.com
#
# 可用环境变量：REGISTRY、IMAGE、PLATFORM、GOPROXY、VERSION
#              以非 linux/amd64 推送需要额外设置 ALLOW_FOREIGN_PLATFORM=1

set -euo pipefail

REGISTRY="${REGISTRY:-registry.vxk8s.com}"
IMAGE="${IMAGE:-${REGISTRY}/cfdata-web/cfdata-web}"
PLATFORM="${PLATFORM:-linux/amd64}"
GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$REPO_ROOT"

PUSH=1
while [ $# -gt 0 ]; do
  case "$1" in
    --local) PUSH=0 ;;
    # 打印文件顶部连续的注释块，不做行号假设，改注释不会让它失效
    -h|--help) awk 'NR>1 && /^#/ {sub(/^# ?/, ""); print; next} NR>1 {exit}' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "未知参数: $1（可用：--local）" >&2; exit 2 ;;
  esac
  shift
done

# 版本号默认取最近的 git tag。
# 特意不用 git describe --dirty：它只认已跟踪文件的改动，纯新增文件不会带上后缀，
# 于是「新增了几个文件」的改动会打出与已发布版本同名、内容却不同的镜像。这里显式查一次工作区。
if [ -z "${VERSION:-}" ]; then
  VERSION="$(git describe --tags --always 2>/dev/null || echo dev)"
  if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
    VERSION="${VERSION}-dirty"
  fi
fi
REVISION="$(git rev-parse --short HEAD 2>/dev/null || true)"

echo "镜像:   ${IMAGE}"
echo "版本:   ${VERSION}"
echo "架构:   ${PLATFORM}"
if [ "$PUSH" -eq 1 ]; then
  echo "动作:   构建并推送到私有仓库"
else
  echo "动作:   仅构建并载入本机（不推送）"
fi
echo

command -v docker >/dev/null 2>&1 || { echo "错误：未找到 docker 命令" >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo "错误：Docker daemon 未运行，或当前用户无权访问" >&2; exit 1; }
docker buildx version >/dev/null 2>&1 || { echo "错误：未找到 buildx 插件（docker buildx version 执行失败）" >&2; exit 1; }

# 推送会一并覆盖 :latest，而服务器拉的就是 linux/amd64 的 latest。
# 用别的架构推送等于把服务器在用的镜像换成跑不起来的，所以默认拦下来。
if [ "$PUSH" -eq 1 ] && [ "$PLATFORM" != "linux/amd64" ] && [ "${ALLOW_FOREIGN_PLATFORM:-0}" != "1" ]; then
  {
    echo "错误：PLATFORM=${PLATFORM}，但推送会一并覆盖 ${IMAGE}:latest。"
    echo "      服务器拉取的是 linux/amd64，覆盖后会让它拿到跑不起来的镜像。"
    echo "      确实要这样做，请设置 ALLOW_FOREIGN_PLATFORM=1 重跑。"
  } >&2
  exit 1
fi

BUILD_ARGS=(
  --platform "${PLATFORM}"
  --build-arg "VERSION=${VERSION}"
  --build-arg "REVISION=${REVISION}"
  --build-arg "GOPROXY=${GOPROXY}"
  --tag "${IMAGE}:${VERSION}"
  --tag "${IMAGE}:latest"
  # 关掉 provenance / sbom 证言。buildx 默认会额外生成一层 attestation 清单，
  # 推送后是个含 unknown/unknown 条目的 OCI index，部分私有仓库（尤其老版本 Harbor）
  # 界面显示异常，某些客户端拉取也会报 no matching manifest。
  --provenance=false
  --sbom=false
)

if [ "$PUSH" -eq 1 ]; then
  echo "提示：若推送报认证失败，先执行 docker login ${REGISTRY}"
  echo
  docker buildx build "${BUILD_ARGS[@]}" --push .
  echo
  echo "推送完成："
  echo "  ${IMAGE}:${VERSION}"
  echo "  ${IMAGE}:latest"
  echo
  echo "服务器上更新："
  echo "  docker compose pull && docker compose up -d"
else
  docker buildx build "${BUILD_ARGS[@]}" --load .
  echo
  echo "已载入本机镜像，可直接本地验证："
  echo "  docker compose up -d"
fi
