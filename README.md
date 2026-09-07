# llama.cpp Router Monitor

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)](https://www.docker.com/)
[![SQLite](https://img.shields.io/badge/SQLite-local-003B57?logo=sqlite&logoColor=white)](https://www.sqlite.org/)
[![llama.cpp](https://img.shields.io/badge/llama.cpp-local%20LLM-111111)](https://github.com/ggml-org/llama.cpp)
[![Streaming](https://img.shields.io/badge/SSE-streaming-1f6feb)](#what-it-does)

Monitor and inspect all traffic going through your local `llama.cpp` server.

Lightweight reverse proxy and monitoring UI for `llama.cpp` and OpenAI-compatible local inference servers.

It sits in front of your inference server, logs every request, stores raw payloads, measures latency and token throughput, and gives you a live web UI.

## What This Is For

`llama.cpp Router Monitor` is a tool for tracking **all requests to a local LLM** in a way that is easy to inspect, filter, and debug.

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

- Reverse-proxies requests to your `llama.cpp` server
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
http://localhost:9091/_monitor/ui
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
> The binary inside the image is `/app/llama_proxy`.

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
http://localhost:9091/_monitor/ui
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
client -> llama.cpp Router Monitor -> llama.cpp
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

- `monitor.db` - SQLite database
- `raw/YYYY-MM-DD/*.gz` - raw request/response payloads

Old data is deleted automatically after `RETENTION_DAYS`.

If you want to keep everything indefinitely:

```env
RETENTION_DAYS=0
```

`0` or any negative value disables automatic cleanup.

## API Endpoints

- `GET /_monitor/health`
- `GET /_monitor/live`
- `GET /_monitor/stats?hours=24`
- `GET /_monitor/requests?limit=100&offset=0`
- `GET /_monitor/request/{id}`
- `DELETE /_monitor/request/{id}`
- `GET /_monitor/raw/{id}/request`
- `GET /_monitor/raw/{id}/response`
- `GET /_monitor/events`
- `GET /_monitor/backend-metrics?limit=200`
- `GET /_monitor/ui`

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

Main environment variables:

- `LISTEN_ADDR`
- `ALLOW_DYNAMIC_BACKEND`
- `RETENTION_DAYS`
- `MAX_REQUEST_BYTES`
- `MAX_CAPTURE_BYTES`
- `REQUEST_TIMEOUT_SECONDS`
- `POLL_BACKEND_METRICS`
- `POLL_INTERVAL_SECONDS`
- `DATA_DIR`

Backends are configured via `backends.list` in the YAML file.

See [`.env.example`](./.env.example) for defaults.

### YAML Configuration

The monitor supports a YAML configuration file with environment variable overrides.

Set `CONFIG_PATH` (default `config.yaml`) to enable it.

**Priority:** environment variables > YAML file > built-in defaults.

```yaml
server:
  listen_addr: ":9091"
  data_dir: "./data"

database:
  type: "sqlite"              # sqlite | postgresql
  sqlite:
    path: "monitor.db"
  postgresql:
    dsn: "postgres://user:pass@localhost:5432/monitor?sslmode=disable"

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

monitor:
  retention_days: 14
  max_request_bytes: 33554432
  max_capture_bytes: 33554432
  request_timeout_seconds: 600
  poll_backend_metrics: true
  poll_interval_seconds: 10
```

- [Quick Start](config.quickstart.yaml) - minimal configuration
- [Full Example](config.example.yaml) - all available options

### Environment Variables (Legacy)

Environment variables remain fully supported. When both are present, environment variables override the YAML file:

| YAML | Environment Variable | Default |
|------|---------------------|---------|
| `server.listen_addr` | `LISTEN_ADDR` | `:9091` |
| `server.data_dir` | `DATA_DIR` | `./data` |
| `backends.allow_dynamic` | `ALLOW_DYNAMIC_BACKEND` | `true` |
| `monitor.retention_days` | `RETENTION_DAYS` | `14` |
| `monitor.max_request_bytes` | `MAX_REQUEST_BYTES` | `33554432` |
| `monitor.max_capture_bytes` | `MAX_CAPTURE_BYTES` | `33554432` |
| `monitor.request_timeout_seconds` | `REQUEST_TIMEOUT_SECONDS` | `600` |
| `monitor.poll_backend_metrics` | `POLL_BACKEND_METRICS` | `true` |
| `monitor.poll_interval_seconds` | `POLL_INTERVAL_SECONDS` | `10` |

### PostgreSQL Database

The monitor uses SQLite by default. To use PostgreSQL, set env vars:

```env
DATABASE_TYPE=postgresql
DATABASE_DSN=postgres://user:password@localhost:5432/monitor?sslmode=disable
```

or use the `database` section in the YAML file.

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
- `api_key` - the backend's API key. When set, the proxy injects `Authorization: Bearer <api_key>` on the request to that backend. Without it, the client's `Authorization` header passes through unchanged.

Per-request overrides still take priority over the balancer:

- header `X-Backend-URL: http://other-server:8080`
- query parameter `?backend=http://other-server:8080`

## Resource Usage Notes

To keep it lean:

- keep `MAX_CAPTURE_BYTES` reasonable, for example `8MB` to `32MB`
- disable backend metrics polling if you do not need it:
  - `POLL_BACKEND_METRICS=false`
- increase `POLL_INTERVAL_SECONDS` if `/metrics` does not need frequent polling

## Limitations

- some token and timing fields depend on what your backend actually returns
- raw payload capture can use noticeable disk space if retention is high
- this is a lightweight local monitor, not a full observability platform

## Roadmap

- charts for latency and throughput
- easier export of requests and metrics
- optional auth for shared environments
- better comparison across models and backends

## License

MIT License.

You can use, modify, and distribute this project with attribution.

See [LICENSE](./LICENSE).
