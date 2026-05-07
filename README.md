# pkgsitex

[`golang/pkgsite`](https://github.com/golang/pkgsite) 的 fork，加了内网部署 / 私有仓库浏览常用能力——`-base-path` 子路径、未导出符号显示 + 浏览器 toggle、godoc 注释 markdown 扩展（` `code` ` / `**bold**` / mermaid 围栏）、view source 走本地 file mux 等。

> **Fork 维护策略**：master 永远 = upstream master，patches 落在 long-lived branch [`fork/main`](https://github.com/NickWilde18/pkgsitex/tree/fork/main)。GitHub "Sync fork" 永远 fast-forward 0 conflict。所有用法 / 启动 / patch 详见 fork/main 分支的 README。

```sh
git clone https://github.com/NickWilde18/pkgsitex.git ~/Repo/pkgsitex
cd ~/Repo/pkgsitex
git checkout fork/main          # 切到 fork patches 分支
cat README.md                   # ← 你看到的 fork 说明
```

## 快速启动（本地，秒级）

前置：Go 1.24+，本地把要浏览的 repo clone 到 `~/Repo/<name>`。

```sh
go run ./cmd/pkgsite -base-path=/pkgsitex -show-unexported -http=:8089 \
    ~/Repo/Chat \
    ~/Repo/Doubao-Speech-Service \
    ~/Repo/UniAuth/uniauth-gf \
    ~/Repo/UniAuth/ittools_sync \
    ~/Repo/open-platform

open http://localhost:8089/pkgsitex/
```

或者编译一次本地复用：

```sh
go install ./cmd/pkgsite
pkgsite -base-path=/pkgsitex -show-unexported -http=:8089 ~/Repo/Chat ...
```

首次 `go run` 拉 esbuild / safehtml 等 dep 走 GOPROXY，约 30 秒，后续秒启。

## docker-compose 启动（备选，给没 Go toolchain 的同事）

仓库根有 [`compose.yaml`](compose.yaml)：

```sh
docker compose up -d
open http://localhost:8089/pkgsitex/
```

首次 build 拉 `golang:1.24` image（~700 MB） + esbuild bundle，约 3-5 分钟。改 `fork/main` 后 `docker compose build pkgsitex` 增量很快。

## 配置选项

| flag | 用途 | 默认 |
|---|---|---|
| `-base-path=/pkgsitex` | 站点挂子路径下，反代 / 共享域名场景。空 = 挂根（pkg.go.dev 行为） | `""` |
| `-show-unexported` | godoc 渲染保留未导出符号；浏览器用 toggle 控制显隐 | `false` |
| `-http=:8089` | 监听端口 | `localhost:8080` |
| `-cache` | 走 GOMODCACHE | `false` |
| `-proxy` | 走 GOPROXY 拉远程 module | `false` |
| `<path>...` | 要索引的 Go module 路径（多个） | `.` |

完整 flag：`go run ./cmd/pkgsite -h`。

## 增 / 减仓库

**本地 binary**：直接改 command 末尾 path 列表。

**docker-compose**：编辑 [`compose.yaml`](compose.yaml) `volumes:` 加一行 `../<name>:/repos/<name>:ro`，`command:` 末尾加 `/repos/<name>`。

约定：内部仓库都 clone 在 `~/Repo/<name>`。UniAuth 是 monorepo 含 `uniauth-gf/` + `ittools_sync/` 两个 Go module，各 mount 一份——pkgsite 不支持 nested module 自动发现。

## 浏览体验

| 元素 | 行为 |
|---|---|
| URL `/pkgsitex/<module>` | module overview |
| URL `/pkgsitex/<module>/<sub>` | 子包 |
| URL `/pkgsitex/<module>@<tag>` | 指定 git tag（local mode 走 module cache，prod worker 模式才有完整 tag 历史） |
| **Show unexported** button | Index 标题旁，切私有 declaration / 侧边栏 / index 链接的显隐，状态 localStorage 跨页保留 |
| **Show internal directories** button | 上游内置，切 `internal/` 子目录显示。`data-local=true` 时自动开启 |
| ` `code` ` / `**bold**` / ` ```mermaid ` | godoc 注释里这些 markdown 写法都渲染（fork 加的 dochtml ext） |
| **View Source** | 包详情页每个声明右侧链接，跳本地 mount 的源码文件（不依赖 GitHub 在线访问） |

## Patch 集合（fork 改了什么）

- **`-base-path`**：URL 子路径前缀，所有 mux pattern / template helper / godoc cross-reference / view source 自动 prefix
- **`-show-unexported`**：让 `internal/fetch/load.go` 在 AST 阶段保留 unexported FuncDecl + `doc.NewFromFiles` 用 `doc.AllDecls`
- **godoc markdown ext**（`internal/godoc/dochtml/internal/render/markdown_ext.go`）：post-process HTML 加 inline code / bold / mermaid fence 识别
- **mermaid client lazy-load**（`static/frontend/frontend.tmpl`）：页面有 `code.language-mermaid` 才动态 import mermaid@10
- **unexported toggle**（`static/frontend/unit/main/main.ts`）：client-side hide + button + localStorage
- **view source 本地 file mux** + base path 拼接修
- **multi-repo command line**：直接 list 多个 module path
- **trailing slash 死循环修**：`internal/frontend/details.go` 在 base path 模式下区分"挂根"和"base path 自身"

## Prod 部署

Prod 模式跟 dev 不同——跟 pkg.go.dev 自身架构一致 4 件套：

- **postgres**：缓存 module 索引 / 包文档
- **athens**：GOPROXY 缓存 + 用 GitHub PAT 拉私有 repo
- **worker**：异步 fetch module 入 DB
- **frontend**：从 DB 读渲染（端口 8089）

镜像已发布到 docker.io [`nickwilde18/pkgsitex:fork-main`](https://hub.docker.com/r/nickwilde18/pkgsitex)（GitHub Actions 自动 push）。用户不需要 build——拉 image 即用。

```sh
git clone https://github.com/NickWilde18/pkgsitex.git
cd pkgsitex
git checkout fork/main

# 1) 配 PAT
cp deploy/prod/.env.example deploy/prod/.env
$EDITOR deploy/prod/.env             # GITHUB_TOKEN=ghp_xxx

# 2) 编辑要监控的 module 列表（内置 5 个 CUHKSZ 内部 repo 示例）
$EDITOR deploy/prod/config.yaml

# 3) 起 stack（自动拉镜像）
docker compose -f compose.prod.yaml --env-file deploy/prod/.env up -d

# 4) 浏览
open http://localhost:8089/pkgsitex/
```

完整运维（加 / 删 module、周期 cron 刷新、故障排查）：[`deploy/prod/README.md`](deploy/prod/README.md)。

支持 multi-version（git tag 切换）、license permissive（私有 / proprietary 也渲）、配置文件驱动。

## 维护策略

- **master 永远 = upstream master**——不 merge `fork/main` 进 master。GitHub "Sync fork" 永远 fast-forward 0 conflict
- **`fork/main` 是 long-lived patch branch**——所有 fork 改动累积在此分支
- **月度 rebase**：AI 跑 `git rebase upstream/master` 让 `fork/main` 跟随上游新进展。预测 conflict 集中在：
  - `internal/frontend/server.go` 的 mux 列表（上游新加路由时）
  - `static/**/*.tmpl` / `*.ts`（上游改样式 / 加交互时）
  - 其他 fork 文件（如 `markdown_ext.go`、`base-path/base-path.ts`）几乎不会 conflict（fork 独有）

历次 patch 详见 PR [#2](https://github.com/NickWilde18/pkgsitex/pull/2)。

## 故障排查

| 现象 | 排查 |
|---|---|
| `go run` 拉 dep 失败 | `GOPROXY=https://goproxy.cn,direct go run ...`（Dockerfile 默认 goproxy.cn） |
| docker `pull access denied` | 用 `docker compose build pkgsitex` 走本地 build，不要拉 image |
| 浏览 `/pkgsitex/...` 静态资源 404 | 改了 `static/`/模板后没重 build：`go run ./devtools/cmd/static` 重生 bundle，或 docker 路径 `docker compose build pkgsitex` |
| 解析 module 超时 | 走公司 Athens：`docker compose build --build-arg GOPROXY=https://athens.corp.com pkgsitex` 或本地 `export GOPROXY=...` |
| mermaid 不渲染 | 浏览器 console 看 `mermaid load failed`——内网拦了 jsdelivr CDN，需要本地 host mermaid（`third_party/mermaid/` 待 vendor） |
| Sidebar / Index 没"Show unexported"按钮 | 可能 localStorage 之前 toggle key 是老名字（`gogodocs:showUnexported`），点一次按钮即可重置成新 key |

---

## 上游 pkgsite

下游 fork 需要跟随上游进展，保留上游 README 内容如下，方便核对功能 / 升级时定位差异。

# golang.org/x/pkgsite

This repository hosts the source code of the [pkg.go.dev](https://pkg.go.dev) website,
and [`pkgsite`](https://pkg.go.dev/golang.org/x/pkgsite/cmd/pkgsite), a documentation
server program.

[![Go Reference](https://pkg.go.dev/badge/golang.org/x/pkgsite.svg)](https://pkg.go.dev/golang.org/x/pkgsite)

完整上游 README：[golang/pkgsite README](https://github.com/golang/pkgsite#readme)。

## Issues

Fork 自身 issues（base-path / markdown ext / etc）：[NickWilde18/pkgsitex/issues](https://github.com/NickWilde18/pkgsitex/issues)。

上游 pkgsite issues 报到 [`golang/go`](https://golang.org/issues)，前缀 `x/pkgsite:`，详见上游 README。

## Contributing

Fork 内部 PR 投到 `fork/main` 分支（不进 master）。

上游 pkgsite contribution flow（gerrit code review）见 [Contribution Guide](https://golang.org/doc/contribute.html) + [上游 README contributing 段](https://github.com/golang/pkgsite#contributing)。

## License

Unless otherwise noted, the source files are distributed under the BSD-style license found in the LICENSE file.
