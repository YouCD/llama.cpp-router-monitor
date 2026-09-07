FROM node:22-alpine AS frontend
WORKDIR /build/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

FROM golang:1.24-alpine AS build
WORKDIR /src
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN GOPROXY=${GOPROXY} go mod download
COPY . .
COPY --from=frontend /build/web ./web
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embedweb -trimpath -ldflags "-s -w" -o /out/llama_proxy .

FROM alpine:3.21
RUN adduser -D -H -u 10001 app && mkdir -p /app/data && chown -R app:app /app
USER app
WORKDIR /app
COPY --from=build /out/llama_proxy /app/llama_proxy
EXPOSE 9091
ENTRYPOINT ["/app/llama_proxy"]
