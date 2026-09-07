# llama.cpp Router Monitor
#
# 两种构建模式:
#   make build         纯二进制（不嵌入前端，前端从 web/ 目录读取，部署需带上 web/）
#   make build-embed   嵌入模式（先构建前端，再把 web/ 内嵌进二进制，单文件部署）

BINARY       := bin/llama_proxy
EMBED_BINARY := bin/llama_proxy_embed
GO           := go
COMPOSE      := docker compose
VERSION      ?= $(shell git describe --tags --always 2>/dev/null || echo dev)

.PHONY: all build build-embed run test vet fmt fmt-check clean \
        frontend-build docker-build docker-up docker-down help

all: fmt vet build

frontend-build:
	cd frontend && npm run build

# 纯二进制（不嵌入前端）
build:
	mkdir -p bin
	$(GO) build -trimpath -ldflags "-s -w" -o $(BINARY) .

# 嵌入前端：先构建前端再编译，前端随二进制分发
build-embed: frontend-build
	mkdir -p bin
	$(GO) build -tags embedweb -trimpath -ldflags "-s -w" -o $(EMBED_BINARY) .

run: build
	./$(BINARY)

test:
	$(GO) test -v -timeout 120s ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

fmt-check:
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "以下文件需要 gofmt 格式化:"; \
		echo "$$files"; \
		exit 1; \
	fi

clean:
	rm -rf bin

docker-build:
	$(COMPOSE) build

docker-up:
	$(COMPOSE) up -d --build

docker-down:
	$(COMPOSE) down

help:
	@echo "可用目标:"
	@echo "  make build          编译纯二进制到 bin/llama_proxy（前端需带 web/ 目录）"
	@echo "  make build-embed    构建前端并嵌入二进制，输出 bin/llama_proxy_embed（单文件）"
	@echo "  make run            构建纯二进制并运行"
	@echo "  make frontend-build 仅构建前端到 web/"
	@echo "  make test           运行全部测试"
	@echo "  make vet            静态检查"
	@echo "  make fmt            格式化代码"
	@echo "  make fmt-check      检查格式（CI 用）"
	@echo "  make clean          清理产物"
	@echo "  make docker-build   构建 Docker 镜像"
	@echo "  make docker-up      启动 Docker Compose 服务"
	@echo "  make docker-down    停止 Docker Compose 服务"
