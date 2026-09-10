# llama_proxy

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)](https://www.docker.com/)
[![SQLite](https://img.shields.io/badge/SQLite-local-003B57?logo=sqlite&logoColor=white)](https://www.sqlite.org/)
[![llama.cpp](https://img.shields.io/badge/llama.cpp-local%20LLM-111111)](https://github.com/ggml-org/llama.cpp)
[![Streaming](https://img.shields.io/badge/SSE-streaming-1f6feb)](#what-it-does)

Proxy and inspect all traffic going through your local `llama.cpp` server.

Lightweight reverse proxy and dashboard for `llama.cpp` and OpenAI-compatible local inference servers.

It sits in front of your inference server, logs every request, stores raw payloads, measures latency and token throughput, and gives you a live web UI.

## What This Is For

`llama_proxy` is a tool for tracking **all requests to a local LLM** in a way that is easy to inspect, filter, and debug.

It is useful when you want to:

- debug local LLM traffic
- inspect prompts, streaming responses, timings, and token usage
- understand how agents behave step by step
- compare models, settings, and backends
- review failures and slow requests without digging through raw logs

## Screenshot

<!-- Replace this with a real PNG screenshot before publishing -->
![Screenshot](./docs/screenshot.png)

<!-- Inspector view -->
![Inspector](./docs/screenshot-inspector.png)

## What It Does

- Reverse-proxies requests to your `llama.cpp` backend
- Captures request and response payloads
- Stores request history in SQLite
- Tracks:
  - active connections
  - TTFT
  - total latency
  - prompt/output token counts
  - prompt/output tokens per second
  - request/response sizes
  - errors
- Supports streaming and non-streaming responses
- Includes a live web UI with filtering and request inspection
- Cleans up old data automatically

## Why It Is Useful

Most local LLM setups tell you whether a request succeeded, but not **how** it behaved.

This project is meant to answer questions like:

- What exactly did the client send?
- What did the model return, including streaming output?
- How many prompt and output tokens were used?
- How fast was prompt ingestion vs output generation?
- Which requests were slow, failed, or behaved unexpectedly?
- What are my agents actually doing over time?

## Who It Is For

- people running `llama.cpp` locally
- developers building agent workflows
- anyone debugging prompts, tool calls, or request chains
- self-hosters who want visibility without adding heavy infrastructure

## Quick Start

### 1. Clone the repo

```bash
git clone https://github.com/dannychirkov/llama.cpp-router-monitor
cd llama-cpp-router-monitor
```

### 2. Start it

Create a `config.yaml` that points at your `llama.cpp` server:

```yaml
backends:
  list:
    - name: "backend-1"
      url: "http://host.docker.internal:8080"
      weight: 1
      enabled: true
```

Then:

```bash
docker compose up -d --build
```

### 3. Open the UI

```text
http://localhost:9091/_proxy/ui
```

### 4. Point your client to the proxy

Instead of sending requests directly to `llama.cpp`:

- old: `http://localhost:8080`
- new: `http://localhost:9091`

## Use a Pre-built Docker Image (GitHub Container Registry)

Every push to `main` and every `v*` tag triggers an automatic build that pushes
a multi-arch image to **GHCR** (`ghcr.io/<owner>/llama.cpp-router-monitor`).

Available tags:

| Tag            | Description                              |
|----------------|------------------------------------------|
| `next`         | Latest build of the `main` branch        |
| `sha-<commit>` | Exact build for a specific commit        |
| `1.2.3`, `1.2` | Versioned releases (from `v1.2.3` tags)  |
| `latest`       | Same as the most recent release `v*` tag |

### 1. Pull and run

```bash
docker pull ghcr.io/<owner>/llama.cpp-router-monitor:next

docker run -d --name llama-cpp-router-monitor \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/data:/app/data \
  -p 9091:9091 \
  --restart unless-stopped \
  ghcr.io/<owner>/llama.cpp-router-monitor:next
```

> The image is built with the frontend embedded — no separate `web/` needed.
> The binary inside the image is `/app/`.

### 2. Configure backends

Point the proxy at your `llama.cpp` server in `config.yaml`. When running on the
host from a container, use `host.docker.internal`:

```yaml
backends:
  list:
    - name: "backend-1"
      url: "http://host.docker.internal:8080"
      weight: 1
      enabled: true
```

### 3. Open the UI

```text
http://localhost:9091/_proxy/ui
```

> The GHCR package is **private** by default after the first build. To pull it
> from other machines, open the package settings on GitHub and set it to **Public**,
> or authenticate with `docker login ghcr.io` using a token that has `read:packages`.

## Build the Image Yourself

Prefer building from source? The Dockerfile is multi-stage and self-contained
(it builds the frontend and compiles an embedded binary):

```bash
# with docker compose
docker compose up -d --build

# or directly
docker build -t llama-cpp-router-monitor .
```

> In China, builds may fail fetching Go modules / npm packages due to network.
> To use a China mirror, pass build args (works with `docker compose build` too):

```bash
docker build \
  --build-arg GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct \
  -t llama-cpp-router-monitor .
```

## Minimal Configuration

Backends are defined in `backends.list` (see [YAML Configuration](#yaml-configuration)). Minimal `config.yaml`:

```yaml
server:
  listen_addr: ":9091"
  data_dir: "./data"

database:
  type: "sqlite"

backends:
  list:
    - name: "backend-1"
      url: "http://host.docker.internal:8080"
      weight: 1
      enabled: true
```

## Windows Autostart

The container uses:

```yaml
restart: unless-stopped
```

So it comes back automatically when Docker starts.

To start it with Windows:

1. Open Docker Desktop
2. Go to `Settings -> General`
3. Enable `Start Docker Desktop when you sign in`

## One-Line Install For Existing Docker Users

```bash
git clone https://github.com/dannychirkov/llama.cpp-router-monitor && cd llama-cpp-router-monitor && docker compose up -d --build
```

## Runtime Model

The proxy keeps your existing API flow:

```text
client -> llama_proxy -> llama.cpp
```

It does not replace your inference server. It only sits in front of it.

## Privacy

This project is designed for local use.

- requests and responses are stored on your machine
- SQLite and raw payload files stay in `./data`
- nothing is sent anywhere unless you expose the service yourself

## Dynamic Backend Override

Requests are routed to backends defined in `backends.list` (weighted load balancing).

You can override the backend per request (requires `allow_dynamic: true`) with:

- header:

```text
X-Backend-URL: http://host.docker.internal:8081
```

- or query parameter:

```text
?backend=http://host.docker.internal:8081
```

## Data Storage

Local data is stored in `./data`:

- `proxy.db` - SQLite database
- `raw/YYYY-MM-DD/*.gz` - raw request/response payloads

The data is cleaned automatically after `retention_days`.

If you want to keep everything indefinitely, set `retention_days: 0` in the YAML config:

```yaml
monitor:
  retention_days: 0
```

`0` or any negative value disables automatic cleanup.

## API Endpoints

- `GET /health`
- `GET /live`
- `GET /stats?hours=24`
- `GET /requests?limit=100&offset=0`
- `GET /request/{id}`
- `DELETE /request/{id}`
- `GET /raw/{id}/request`
- `GET /raw/{id}/response`
- `GET /events`
- `GET /backend-metrics?limit=200`
- `GET /ui`
- `GET /v1/models` — OpenAI-compatible model list, answered by the router itself (not forwarded)

Supported request filters:

- `q`
- `path`
- `model`
- `method`
- `status`
- `since_hours`
- `stream`
- `errors_only`
- `with_tokens`

## Example Request

```bash
curl http://localhost:9091/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"local-model","messages":[{"role":"user","content":"hi"}],"stream":false}'
```

## Configuration

This proxy is configured exclusively via a YAML configuration file.

Backends are configured via `backends.list` in the YAML file.

### YAML Configuration

Pass the config file with `-f <path>` (default `config.yaml`).

```yaml
server:
  listen_addr: ":9091"
  data_dir: "./data"

database:
  type: "sqlite"              # sqlite | postgresql
  sqlite:
    path: "proxy.db"
  postgresql:
    dsn: "postgres://user:pass@localhost:5432/proxy?sslmode=disable"

backends:
  allow_dynamic: true
  strategy: "wrr"             # wrr | swrr | wr | rr | random
  list:
    - name: "gpu-1"
      url: "http://gpu-server-1:8080"
      weight: 50
      enabled: true
      model: "qwen"           # 后端实际部署的模型 ID，转发时自动重写请求体的 model
      api_key: "secret-1"     # 该后端的 API Key，自动注入 Authorization: Bearer <key>
    - name: "gpu-2"
      url: "http://gpu-server-2:8080"
      weight: 30
      enabled: true
      model: "deepseek"       # 后端实际部署的模型 ID
      api_key: "secret-2"

proxy:
  api_key: "client-secret"    # 客户端访问 proxy 的 API Key（与后端 api_key 隔离）
  retention_days: 14
  max_request_bytes: 33554432
  max_capture_bytes: 33554432
  request_timeout_seconds: 600
  poll_backend_metrics: true
  poll_interval_seconds: 10
```

- [Quick Start](config.quickstart.yaml) - minimal configuration
- [Full Example](config.example.yaml) - all available options

### PostgreSQL Database

This proxy uses SQLite by default. To use PostgreSQL, use the `database` section in the YAML file:

```yaml
database:
  type: postgresql
  postgresql:
    dsn: "postgres://user:password@localhost:5432/proxy?sslmode=disable"
```

On startup the monitor:
- **auto-creates the database itself** if it does not exist (connects to the `postgres` maintenance database and runs `CREATE DATABASE`; the DB user needs `CREATEDB` privilege)
- creates all tables and indexes automatically (`CREATE TABLE IF NOT EXISTS`)

### Multi-Backend Load Balancing

When `backends.list` is configured, requests are routed across enabled backends using a weighted strategy (using [`fufuok/balancer`](https://github.com/fufuok/balancer)):

- `wrr` - weighted round robin (default)
- `swrr` - smooth weighted round robin
- `wr` - weighted random
- `rr` - round robin
- `random` - random

Per-backend options:

- `model` - the model ID the backend actually serves. When set, the proxy rewrites the `model` field in the request body to this value before forwarding. Example: backend `gpu-server-1` serves `qwen`, `gpu-server-2` serves `deepseek` - each request is rewritten to the model of the chosen backend.
- `api_key` - the backend's API key. When set, the proxy injects `Authorization: Bearer <api_key}` on the request to that backend. Without it, the client's `Authorization` header passes through unchanged.

### 客户端 API Key（`proxy.api_key`）

在 `proxy.api_key` 配置后，所有客户端请求需在 header 中携带 `Authorization: Bearer <key>`，否则返回 401。该 key 与后端 `api_key` 相互隔离，互不依赖。

- 不配置（空值）→ 无客户端鉴权（兼容旧版行为）
- 配置了 → 客户端必须携带正确 API Key

**客户端请求示例：**

```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Authorization: Bearer client-secret-2024" \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen3.8","messages":[{"role":"user","content":"hi"}]}'
```

**代理内部隔离：**

```
客户端 ── Bearer <client_key> ──► llm_proxy ── Bearer <backend_key> ──► 后端 LLM
```

Per-request overrides still take priority over the balancer:

- header `X-Backend-URL: http://other-server:8080`
- query parameter `?backend=http://other-server:8080`

## Resource Usage Notes

To keep it lean:

- keep `proxy.max_capture_bytes` reasonable, for example `8MB` to `32MB`
- disable backend metrics polling if you do not need it: `proxy.poll_backend_metrics: false`
- increase `proxy.poll_interval_seconds` if `/metrics` does not need frequent polling

## Limitations

- some token and timing fields depend on what your backend actually returns
- raw payload capture can use noticeable disk space if retention is high
- this is a lightweight local proxy, not a full observability platform

## Roadmap

- charts for latency and throughput
- easier export of requests and metrics
- optional auth for shared environments
- better comparison across models and backends

## License

MIT License.

You can use, modify, and distribute this project with attribution.

See [LICENSE](./LICENSE).
