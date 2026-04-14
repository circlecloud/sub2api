#!/usr/bin/env bash
# 本地构建镜像的快速脚本，避免在命令行反复输入构建参数。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION="${VERSION:-$(tr -d '\r\n' < "${REPO_ROOT}/backend/cmd/server/VERSION")}"
COMMIT_SHORT="${COMMIT_SHORT:-$(git -C "${REPO_ROOT}" rev-parse --short=8 HEAD)}"
TIME_TAG="${TIME_TAG:-$(date +%y%m%d%H%M%S)}"
DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
DOCKER_REPO="${DOCKER_REPO:-docker.c5mc.cn/weishaw/sub2api}"
NODE_IMAGE="${NODE_IMAGE:-docker.1ms.run/library/node:24-alpine}"
GOLANG_IMAGE="${GOLANG_IMAGE:-docker.1ms.run/library/golang:1.26.2-alpine}"
ALPINE_IMAGE="${ALPINE_IMAGE:-docker.1ms.run/library/alpine:3.21}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-docker.1ms.run/library/postgres:18-alpine}"
GOPROXY_VALUE="${GOPROXY:-https://goproxy.cn,direct}"
GOSUMDB_VALUE="${GOSUMDB:-sum.golang.google.cn}"
DOCKER_BUILDX_MAX_USED_SPACE="${DOCKER_BUILDX_MAX_USED_SPACE:-15gb}"
DOCKER_SKIP_LOCAL_TAG_PRUNE="${DOCKER_SKIP_LOCAL_TAG_PRUNE:-0}"
DOCKER_SKIP_BUILDX_PRUNE="${DOCKER_SKIP_BUILDX_PRUNE:-0}"

CURRENT_TAGS=(
    "${DOCKER_REPO}:${VERSION}-local"
    "${DOCKER_REPO}:${VERSION}-${COMMIT_SHORT}-local"
    "${DOCKER_REPO}:${VERSION}-${TIME_TAG}-local"
    "${DOCKER_REPO}:latest"
)

cleanup_old_local_tags() {
    if [ "${DOCKER_SKIP_LOCAL_TAG_PRUNE}" = "1" ]; then
        return
    fi
    while IFS= read -r tag; do
        [ -n "${tag}" ] || continue
        case "${tag}" in
            "${DOCKER_REPO}:"*-local|"${DOCKER_REPO}:latest")
                keep=0
                for current in "${CURRENT_TAGS[@]}"; do
                    if [ "${tag}" = "${current}" ]; then
                        keep=1
                        break
                    fi
                done
                if [ "${keep}" = "0" ]; then
                    docker image rm "${tag}" >/dev/null 2>&1 || true
                fi
                ;;
        esac
    done < <(docker image ls "${DOCKER_REPO}" --format '{{.Repository}}:{{.Tag}}' 2>/dev/null || true)

    docker image prune -f --filter "label=org.opencontainers.image.source=https://github.com/Wei-Shaw/sub2api" >/dev/null 2>&1 || true
}

prune_buildx_cache() {
    if [ "${DOCKER_SKIP_BUILDX_PRUNE}" = "1" ]; then
        return
    fi
    docker buildx version >/dev/null 2>&1 || return 0
    docker buildx prune -f --max-used-space "${DOCKER_BUILDX_MAX_USED_SPACE}" >/dev/null 2>&1 || true
}

docker build \
    -t "${CURRENT_TAGS[0]}" \
    -t "${CURRENT_TAGS[1]}" \
    -t "${CURRENT_TAGS[2]}" \
    -t "${CURRENT_TAGS[3]}" \
    --build-arg NODE_IMAGE="${NODE_IMAGE}" \
    --build-arg GOLANG_IMAGE="${GOLANG_IMAGE}" \
    --build-arg ALPINE_IMAGE="${ALPINE_IMAGE}" \
    --build-arg POSTGRES_IMAGE="${POSTGRES_IMAGE}" \
    --build-arg GOPROXY="${GOPROXY_VALUE}" \
    --build-arg GOSUMDB="${GOSUMDB_VALUE}" \
    --build-arg VERSION="${VERSION}" \
    --build-arg COMMIT="${COMMIT_SHORT}" \
    --build-arg DATE="${DATE}" \
    -f "${REPO_ROOT}/Dockerfile" \
    "${REPO_ROOT}"

cleanup_old_local_tags
prune_buildx_cache

printf 'repo=%s\nversion_tag=%s\ncommit_tag=%s\ntime_tag=%s\nlatest_tag=%s\nbuildx_max_used_space=%s\n' \
    "${DOCKER_REPO}" \
    "${CURRENT_TAGS[0]}" \
    "${CURRENT_TAGS[1]}" \
    "${CURRENT_TAGS[2]}" \
    "${CURRENT_TAGS[3]}" \
    "${DOCKER_BUILDX_MAX_USED_SPACE}"
