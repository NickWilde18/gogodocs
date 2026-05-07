# GoGoDocs（fork from golang/pkgsite）容器镜像。
#
# 目标场景：内网部署 cmd/pkgsite local mode 服务多个 mount 进来的 Go module
# 仓库（不走上游 cmd/frontend + worker + Postgres 的全套，省一个数量级运维）。
#
# 不能用 alpine 极简 base——cmd/pkgsite 在 local mode 下通过 go/packages.Load
# 调 `go list` 解析模块依赖，container 必须自带 Go toolchain。同 base image
# 用 golang:1.24 既 build 又 run，stage 1 复制二进制到 stage 2 也只是少几 MB
# 收益不显著。
#
# GOPROXY 默认走 goproxy.cn 应对国内网络环境；改 build arg / env 可换走
# 公司内网 Athens 之类。

FROM golang:1.24 AS builder
WORKDIR /src

# go.mod / go.sum 单独 COPY 让 dep 下载层可缓存（跟随 source 改动）
COPY go.mod go.sum ./
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /pkgsite ./cmd/pkgsite

FROM golang:1.24
LABEL org.opencontainers.image.source="https://github.com/NickWilde18/pkgsitex"
LABEL org.opencontainers.image.description="Self-hosted godoc browser (pkgsite fork) with base-path support"

COPY --from=builder /pkgsite /usr/local/bin/pkgsite

# runtime 也需要 GOPROXY 给 go/packages 拉依赖（即便 mount 进来的仓库是 vendor 模式）
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

# /repos 是约定的 mount 目录，docker-compose 把每个 Go module 仓库挂进来
WORKDIR /repos

EXPOSE 8080

# 默认参数让 -h 能直接看 usage；docker-compose 通过 command: 覆盖
ENTRYPOINT ["pkgsite"]
CMD ["-h"]
