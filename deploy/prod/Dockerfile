# pkgsitex prod 三进程镜像（worker / frontend / init）共享 base，build 一次
# 输出三种 target，docker-compose 用 build.target 选。
#
# 不同于 cmd/pkgsite local mode 的 Dockerfile（仓库根 Dockerfile）：那个跑
# fetchdatasource 直读文件系统，不走 DB 不走 GOPROXY；本镜像跑 cmd/worker
# + cmd/frontend，需要 PostgreSQL + athens GOPROXY，是 pkg.go.dev 自身的
# prod 架构。

FROM golang:1.24 AS builder
WORKDIR /src

COPY go.mod go.sum ./
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/worker ./cmd/worker
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/frontend ./cmd/frontend
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/pkgsitex-init ./cmd/pkgsitex-init

# 共享 base：ca-certificates 给 HTTPS / git；git 给 cmd/pkgsitex-init 跑
# ls-remote 拿 tag 列表（worker / frontend 不直接调 git，但镜像统一一份）
FROM debian:bookworm-slim AS base
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates git && \
    rm -rf /var/lib/apt/lists/*

# === target: worker（跑 cmd/worker 拉 module 入 DB）===
FROM base AS worker
COPY --from=builder /bin/worker /usr/local/bin/worker
COPY --from=builder /src/static /app/static
COPY --from=builder /src/migrations /app/migrations
WORKDIR /app
EXPOSE 8000
ENTRYPOINT ["worker"]

# === target: frontend（跑 cmd/frontend 渲染 godoc）===
FROM base AS frontend
COPY --from=builder /bin/frontend /usr/local/bin/frontend
COPY --from=builder /src/static /app/static
COPY --from=builder /src/third_party /app/third_party
WORKDIR /app
EXPOSE 8080
ENTRYPOINT ["frontend"]

# === target: init（跑 cmd/pkgsitex-init 写 netrc + enqueue 全 module）===
FROM base AS init
COPY --from=builder /bin/pkgsitex-init /usr/local/bin/pkgsitex-init
ENTRYPOINT ["pkgsitex-init"]
