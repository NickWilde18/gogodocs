# pkgsitex prod 单 image 三 entrypoint（worker / frontend / init）。
#
# 单 image 设计：减少 docker.io push 频次和拉取流量——用户拉一次就拿到三个
# 二进制；compose 里通过 entrypoint: 选择跑哪个。三种角色共享一份 ca-certs
# 和 git，本来分开 image 也是同 base，单 image 没浪费。
#
# 跟仓库根 `Dockerfile`（cmd/pkgsite local mode）不同：那个跑 fetchdatasource
# 直读文件系统，不走 DB 不走 GOPROXY；本镜像跑 cmd/worker / cmd/frontend，
# 需要 PostgreSQL + athens GOPROXY，跟 pkg.go.dev 自身架构一致。

FROM golang:1.25 AS builder
WORKDIR /src

COPY go.mod go.sum ./
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/worker ./cmd/worker
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/frontend ./cmd/frontend
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/pkgsitex-init ./cmd/pkgsitex-init

FROM debian:bookworm-slim
LABEL org.opencontainers.image.source="https://github.com/NickWilde18/pkgsitex"
LABEL org.opencontainers.image.description="Self-hosted pkgsite fork (worker/frontend/init in one image)"
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates git wget && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /bin/worker /usr/local/bin/worker
COPY --from=builder /bin/frontend /usr/local/bin/frontend
COPY --from=builder /bin/pkgsitex-init /usr/local/bin/pkgsitex-init

# 静态资源 / migrations / third_party 都内置——cmd/worker / cmd/frontend 都
# 通过 -static / -third_party flag 指 /app 下，cmd/worker 内部跑 migrations
# 时也能找到 /app/migrations
COPY --from=builder /src/static /app/static
COPY --from=builder /src/third_party /app/third_party
COPY --from=builder /src/migrations /app/migrations
WORKDIR /app

# 默认 entrypoint = frontend（用户最常用）；compose 里其他 service 用
# entrypoint: ["/usr/local/bin/worker"] / ["/usr/local/bin/pkgsitex-init"] 覆盖
EXPOSE 8000 8080
ENTRYPOINT ["/usr/local/bin/frontend"]
