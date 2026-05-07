# pkgsitex prod 部署

完整 4 件套 stack（postgres + athens + worker + frontend）+ init / migrate 一次性容器，跑配置驱动 + 私有 repo 自动 fetch + multi-version 支持。

跟仓库根的 `compose.yaml`（cmd/pkgsite local mode，单进程，文件 mount）不同——本目录是 prod 模式，跟 pkg.go.dev 自身架构一致。

## 启动步骤

```sh
cd ~/Repo/pkgsitex     # 仓库根，不是 deploy/prod/

# 1) 配 PAT
cp deploy/prod/.env.example deploy/prod/.env
$EDITOR deploy/prod/.env             # GITHUB_TOKEN=ghp_xxx

# 2) 配要监控的 module 列表
$EDITOR deploy/prod/config.yaml      # modules: [...]

# 3) 起 stack（首次 build ~10 分钟下 image + 编译）
docker compose -f compose.prod.yaml --env-file deploy/prod/.env up -d

# 4) 看日志确认 worker 在拉 module
docker compose -f compose.prod.yaml logs -f worker

# 5) 浏览
open http://localhost:8089/pkgsitex/
```

## 运维操作

### 加 / 删 module

编辑 [`config.yaml`](config.yaml) 后触发 init 一次性容器重新 enqueue：

```sh
docker compose -f compose.prod.yaml --env-file deploy/prod/.env \
    run --rm init --mode=refresh
```

`refresh` 模式跳过 netrc 写 + worker wait（stack 已稳定运行），仅重 ls-remote tag + enqueue 新版本给 worker。

### 周期刷新（cron）

每天凌晨 3 点跑 `init --mode=refresh` 增量拉新版本。host crontab：

```
0 3 * * * cd ~/Repo/pkgsitex && docker compose -f compose.prod.yaml \
          --env-file deploy/prod/.env run --rm init --mode=refresh
```

或者用 host systemd timer。

### 看 athens 缓存了哪些 module

```sh
docker compose -f compose.prod.yaml exec athens ls /var/lib/athens
```

### 重置（清 DB + athens 缓存）

```sh
docker compose -f compose.prod.yaml down -v   # -v 删 volumes
```

## 架构

```
              ┌──────────────────┐
   配置文件   │ config.yaml      │  modules + token + worker URL
              └────────┬─────────┘
                       │ read
                       ▼
              ┌──────────────────┐
              │  init container  │  写 netrc + git ls-remote --tags +
              │ (one-shot)       │  POST worker /fetch/<mod>/@v/<ver>
              └────────┬─────────┘
                       │
                       ▼
       ┌──────┐  GOPROXY  ┌────────────┐  fetch+parse  ┌────────┐
       │worker├──────────►│   athens   │──── git ────► │ GitHub │
       └──┬───┘           │ (PAT auth) │     PAT       │ 私有   │
          │ store         └────────────┘               └────────┘
          ▼
       ┌────────┐
       │postgres│ ◄────── frontend（端口 8089）─── 用户浏览
       └────────┘
```

数据流（首次访问 `module@v1.2.3`）：

1. 用户浏览器 → frontend 端口 8089
2. frontend 查 postgres 没有此 module 数据 → 返 "fetching" 页面
3. （并行）init 容器启动时已 enqueue（worker `/fetch/...`），worker 异步处理
4. worker 调 athens GOPROXY 协议要 module zip
5. athens 第一次见 → git clone 用 PAT → 缓存到 disk → 返 zip
6. worker 解析 zip → 入 postgres
7. 用户刷新 → frontend 命中 postgres → 渲 godoc

## 配置文件 schema

[`config.yaml`](config.yaml) 字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `github.token` | string | classic GitHub PAT（repo scope full）；建议写 `${GITHUB_TOKEN}` 走 env |
| `worker.url` | string | worker 内部 URL，docker-compose 默认 `http://worker:8000` |
| `athens.netrcPath` | string | init 写 netrc 给 athens 用，默认 `/etc/athens/.netrc` |
| `modules[].path` | string | Go module path |
| `modules[].versions` | string | `latest`（默认） / `all-tags` |
| `modules[].repoUrl` | string | 可选——monorepo 子模块时指定 git repo URL（不含子目录） |

## License

prod stack frontend 默认开 `licenses.permissive`——所有 module 不论 license 类型都渲染（包括 `All Rights Reserved` / unknown / proprietary）。这是内网部署诉求；不希望渲染某些 module 时把它从 `config.yaml` 删除即可。

## 故障排查

| 现象 | 排查 |
|---|---|
| `worker` 容器拉 module 401 | `docker logs athens` 看是否 git clone 401，PAT 错或 scope 不够 |
| init 容器 git ls-remote `Permission denied` | 同上，PAT 没 `repo` scope |
| frontend 页面 "this package is not redistributable" | 检查 frontend 容器 env `PKGSITEX_LICENSE_PERMISSIVE=true`（cmd/frontend flag 集成完后生效）|
| migrate 失败 | `docker compose logs migrate` 看 SQL 错；通常是 schema 跟上游 mismatch（rebase 后 migrations 文件冲突）|
| athens 拉私有 repo 超时 | 内网 git 出口受限——把 athens 的 git config 走公司代理：编辑 athens 容器 `~/.gitconfig` 加 `http.proxy` |

## TODO

- [ ] cmd/frontend / cmd/worker 加 `-base-path` / `-show-unexported` / `-license-permissive` flag（当前用 env var 占位，未通到代码层）
- [ ] init container 周期 cron 内置（不靠 host crontab）
- [ ] athens config volume 改成 ConfigMap 形式让 helm chart 复用更方便
