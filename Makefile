.PHONY: build build-backend build-frontend sync-local-container docker-build-c5mc docker-push-c5mc docker-prune-local build-datamanagementd test test-backend test-frontend test-datamanagementd secret-scan

LOCAL_CONTAINER ?= sub2api-local
LOCAL_BINARY ?= /tmp/sub2api-local-hot
LOCAL_HEALTH_URL ?= http://127.0.0.1:18081/
LOCAL_GOCACHE ?= /tmp/sub2api-gocache/sync-local-container
LOCAL_FRONTEND_STAMP ?= /tmp/sub2api-frontend-sync.stamp
FORCE_FRONTEND ?= 0
DOCKER_REPO ?= docker.c5mc.cn/weishaw/sub2api
DOCKER_VERSION ?= $(shell tr -d '\r\n' < backend/cmd/server/VERSION)
DOCKER_COMMIT ?= $(shell git rev-parse --short=8 HEAD)
DOCKER_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DOCKER_TIME ?= $(shell date +%y%m%d%H%M%S)
DOCKER_VERSION_TAG ?= $(DOCKER_VERSION)-local
DOCKER_COMMIT_TAG ?= $(DOCKER_VERSION)-$(DOCKER_COMMIT)-local
DOCKER_TIME_TAG ?= $(DOCKER_VERSION)-$(DOCKER_TIME)-local
DOCKER_VERSION := $(DOCKER_VERSION)
DOCKER_COMMIT := $(DOCKER_COMMIT)
DOCKER_DATE := $(DOCKER_DATE)
DOCKER_TIME := $(DOCKER_TIME)
DOCKER_VERSION_TAG := $(DOCKER_VERSION_TAG)
DOCKER_COMMIT_TAG := $(DOCKER_COMMIT_TAG)
DOCKER_TIME_TAG := $(DOCKER_TIME_TAG)
NODE_IMAGE ?= docker.1ms.run/library/node:24-alpine
GOLANG_IMAGE ?= docker.1ms.run/library/golang:1.26.2-alpine
ALPINE_IMAGE ?= docker.1ms.run/library/alpine:3.21
POSTGRES_IMAGE ?= docker.1ms.run/library/postgres:18-alpine
GOPROXY ?= https://goproxy.cn,direct
GOSUMDB ?= sum.golang.google.cn
DOCKER_BUILDX_MAX_USED_SPACE ?= 15gb
DOCKER_SKIP_LOCAL_TAG_PRUNE ?= 0
DOCKER_SKIP_BUILDX_PRUNE ?= 0

# 一键编译前后端
build: build-backend build-frontend

# 编译后端（复用 backend/Makefile）
build-backend:
	@$(MAKE) -C backend build

# 编译前端（完整校验 + 打包）
build-frontend:
	@pnpm --dir frontend run build

# 本地容器同步使用的快速前端打包（跳过 vue-tsc）
build-frontend-sync:
	@pnpm --dir frontend run build:sync

# 本地构建并同步到运行中的本地容器，不额外保留容器备份
sync-local-container:
	@set -eu; \
	mkdir -p "$(LOCAL_GOCACHE)"; \
	mkdir -p "$$(dirname "$(LOCAL_FRONTEND_STAMP)")"; \
	cleanup() { \
		rm -f "$(LOCAL_BINARY)"; \
	}; \
	trap cleanup EXIT INT TERM; \
	FRONTEND_HASH="$$(find frontend/src frontend/public -type f -print 2>/dev/null; printf '%s\n' frontend/index.html frontend/package.json frontend/pnpm-lock.yaml frontend/vite.config.ts frontend/tsconfig.json frontend/tsconfig.node.json | while read -r file; do [ -f "$$file" ] && printf '%s\n' "$$file"; done | sort | xargs sha256sum | sha256sum | awk '{print $$1}')"; \
	PREV_FRONTEND_HASH="$$(cat "$(LOCAL_FRONTEND_STAMP)" 2>/dev/null || true)"; \
	FRONTEND_STATUS="cached"; \
	if [ "$(FORCE_FRONTEND)" = "1" ] || [ ! -f backend/internal/web/dist/index.html ] || [ "$$FRONTEND_HASH" != "$$PREV_FRONTEND_HASH" ]; then \
		$(MAKE) build-frontend-sync; \
		printf '%s\n' "$$FRONTEND_HASH" > "$(LOCAL_FRONTEND_STAMP)"; \
		FRONTEND_STATUS="rebuilt"; \
	fi; \
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOCACHE="$(LOCAL_GOCACHE)" go -C backend build -tags embed -trimpath -ldflags "-s -w -X main.Commit=$$(git rev-parse --short HEAD) -X main.Date=$$(date -u +%Y-%m-%dT%H:%M:%SZ) -X main.BuildType=local" -o "$(LOCAL_BINARY)" ./cmd/server; \
	if [ "$$(docker inspect -f '{{.State.Running}}' "$(LOCAL_CONTAINER)")" = "true" ]; then \
		docker stop "$(LOCAL_CONTAINER)" >/dev/null; \
	fi; \
	docker cp "$(LOCAL_BINARY)" "$(LOCAL_CONTAINER):/app/sub2api"; \
	docker cp backend/resources/. "$(LOCAL_CONTAINER):/app/resources/"; \
	docker start "$(LOCAL_CONTAINER)" >/dev/null; \
	STATUS=""; \
	for i in $$(seq 1 60); do \
		STATUS="$$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$(LOCAL_CONTAINER)")"; \
		if [ "$$STATUS" = "healthy" ]; then \
			break; \
		fi; \
		if [ "$$STATUS" = "exited" ] || [ "$$STATUS" = "dead" ]; then \
			docker logs --tail 100 "$(LOCAL_CONTAINER)"; \
			exit 1; \
		fi; \
		sleep 2; \
	done; \
	test "$$STATUS" = "healthy"; \
	HTTP_CODE="$$(curl -s -o /dev/null -w '%{http_code}' "$(LOCAL_HEALTH_URL)")"; \
	test "$$HTTP_CODE" = "200"; \
	printf 'container=%s\nfrontend=%s\nstatus=%s\nurl=%s\nhttp_code=%s\n' "$(LOCAL_CONTAINER)" "$$FRONTEND_STATUS" "$$STATUS" "$(LOCAL_HEALTH_URL)" "$$HTTP_CODE"

# 构建 docker.c5mc.cn 镜像，并在构建后清理旧的本地标签与过量 BuildKit 缓存
docker-build-c5mc:
	@DOCKER_REPO="$(DOCKER_REPO)" \
		VERSION="$(DOCKER_VERSION)" \
		COMMIT_SHORT="$(DOCKER_COMMIT)" \
		TIME_TAG="$(DOCKER_TIME)" \
		DATE="$(DOCKER_DATE)" \
		NODE_IMAGE="$(NODE_IMAGE)" \
		GOLANG_IMAGE="$(GOLANG_IMAGE)" \
		ALPINE_IMAGE="$(ALPINE_IMAGE)" \
		POSTGRES_IMAGE="$(POSTGRES_IMAGE)" \
		GOPROXY="$(GOPROXY)" \
		GOSUMDB="$(GOSUMDB)" \
		DOCKER_BUILDX_MAX_USED_SPACE="$(DOCKER_BUILDX_MAX_USED_SPACE)" \
		DOCKER_SKIP_LOCAL_TAG_PRUNE="$(DOCKER_SKIP_LOCAL_TAG_PRUNE)" \
		DOCKER_SKIP_BUILDX_PRUNE="$(DOCKER_SKIP_BUILDX_PRUNE)" \
		bash ./deploy/build_image.sh

# 推送 docker.c5mc.cn 镜像，按版本标签、提交标签、时间标签、latest 顺序推送
docker-push-c5mc: docker-build-c5mc
	@set -eu; \
	for tag in "$(DOCKER_VERSION_TAG)" "$(DOCKER_COMMIT_TAG)" "$(DOCKER_TIME_TAG)" latest; do \
		docker push "$(DOCKER_REPO):$$tag"; \
	done

# 清理当前仓库的悬空镜像，并给 BuildKit 缓存设置软上限
docker-prune-local:
	@docker image prune -f --filter "label=org.opencontainers.image.source=https://github.com/Wei-Shaw/sub2api" >/dev/null || true
	@docker buildx prune -f --max-used-space "$(DOCKER_BUILDX_MAX_USED_SPACE)" >/dev/null || true
	@docker system df

# 编译 datamanagementd（宿主机数据管理进程）
build-datamanagementd:
	@cd datamanagement && go build -o datamanagementd ./cmd/datamanagementd

# 运行测试（后端 + 前端）
test: test-backend test-frontend

test-backend:
	@$(MAKE) -C backend test

test-frontend:
	@pnpm --dir frontend run lint:check
	@pnpm --dir frontend run typecheck

test-datamanagementd:
	@cd datamanagement && go test ./...

secret-scan:
	@python3 tools/secret_scan.py
