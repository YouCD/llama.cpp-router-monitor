# llama.cpp Router Monitor

BINARY   := bin/llama_proxy
GO       := go
COMPOSE  := docker compose
VERSION  ?= $(shell git describe --tags --always 2>/dev/null || echo dev)

.PHONY: all build run test vet fmt fmt-check clean \
        docker-build docker-up docker-down help

all: fmt vet build

build:
	mkdir -p bin
	$(GO) build -trimpath -ldflags "-s -w" -o $(BINARY) .

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
	@echo "  make build        编译生成 bin/$(BINARY) 二进制"
	@echo "  make run          编译并运行 $(BINARY)"
	@echo "  make test         运行全部测试"
	@echo "  make vet          静态检查"
	@echo "  make fmt          格式化代码"
	@echo "  make fmt-check    检查格式（CI 用）"
	@echo "  make clean        清理产物"
	@echo "  make docker-build 构建 Docker 镜像"
	@echo "  make docker-up    启动 Docker Compose 服务"
	@echo "  make docker-down  停止 Docker Compose 服务"
