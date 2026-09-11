# ---------- 1. 前端构建 ----------
FROM node:24-alpine AS web

WORKDIR /app/web
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---------- 2. 后端构建 ----------
FROM golang:1.26-alpine AS api

WORKDIR /app

# 容器内直连 proxy.golang.org 常常超时（实测 connection refused），默认走镜像；
# 网络通畅时可用 --build-arg GOPROXY= 置空。必须在 go mod download 之前生效。
ARG GOPROXY=https://goproxy.cn,direct
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOFLAGS=-trimpath \
    GOPROXY=${GOPROXY}

COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN go build -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- 3. 运行 ----------
FROM nginx:1.29-alpine

RUN apk add --no-cache tzdata

WORKDIR /app
COPY --from=api /out/server            /app/server
COPY --from=web /app/web/dist          /usr/share/nginx/html
COPY deploy/nginx.conf                 /etc/nginx/nginx.conf
COPY deploy/entrypoint.sh              /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh && mkdir -p /data

# Nginx 对外 80，Go 只在回环口监听，外部无法绕过闸门直连
EXPOSE 80

ENV ADDR=127.0.0.1:8081 \
    DB_PATH=/data/blog.db \
    REDIS_ADDR=redis:6379 \
    TRUSTED_PROXIES=127.0.0.1,::1 \
    TZ=Asia/Shanghai

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1/healthz || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
