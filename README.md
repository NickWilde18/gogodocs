# pkgsitex

> English ｜ [简体中文](README.zh-CN.md)

A fork of [`golang/pkgsite`](https://github.com/golang/pkgsite) tuned for self-hosting Go module docs in private/internal environments. Adds URL subpath mounting, godoc comment markdown (mermaid / inline code / bold), unexported symbol display + browser toggle, view source via locally mounted files, and config-driven private GitHub repo support.

> **Fork strategy**: master always equals upstream master; all patches live on the long-lived [`fork/main`](https://github.com/NickWilde18/pkgsitex/tree/fork/main) branch. GitHub "Sync fork" stays fast-forward, zero conflict. All usage / setup / patch docs live on `fork/main` — including this README.

```sh
git clone https://github.com/NickWilde18/pkgsitex.git ~/Repo/pkgsitex
cd ~/Repo/pkgsitex
git checkout fork/main          # switch to the fork patches branch
cat README.md                   # ← what you're reading
```

## Quick start (local, sub-second)

Requires Go 1.25+. Clone the repos you want to browse to `~/Repo/<name>`.

```sh
go run ./cmd/pkgsite -base-path=/pkgsitex -show-unexported -http=:8089 \
    ~/Repo/Chat \
    ~/Repo/Doubao-Speech-Service \
    ~/Repo/UniAuth/uniauth-gf \
    ~/Repo/UniAuth/ittools_sync \
    ~/Repo/open-platform

open http://localhost:8089/pkgsitex/
```

Or compile once and reuse locally:

```sh
go install ./cmd/pkgsite
pkgsite -base-path=/pkgsitex -show-unexported -http=:8089 ~/Repo/Chat ...
```

The first `go run` fetches `esbuild` / `safehtml` deps via GOPROXY (~30s); subsequent runs are instant.

## Configuration

| flag | purpose | default |
|---|---|---|
| `-base-path=/pkgsitex` | mount the site under a URL subpath (reverse-proxy / shared-domain scenarios). Empty = mount at root (pkg.go.dev behavior) | `""` |
| `-show-unexported` | godoc renders unexported declarations; the browser uses a toggle to control visibility | `false` |
| `-http=:8089` | listen address | `localhost:8080` |
| `-cache` | use GOMODCACHE | `false` |
| `-proxy` | use GOPROXY to fetch remote modules | `false` |
| `<path>...` | Go module paths to index (multiple) | `.` |

Full flags: `go run ./cmd/pkgsite -h`.

## Adding / removing repos

Edit the path list at the end of the `go run` / `pkgsite` command.

Convention: clone internal repos to `~/Repo/<name>`. UniAuth is a monorepo with two Go modules (`uniauth-gf/` + `ittools_sync/`) — list each separately; pkgsite doesn't auto-discover nested modules.

## Browse experience

| Element | Behavior |
|---|---|
| URL `/pkgsitex/<module>` | module overview |
| URL `/pkgsitex/<module>/<sub>` | subpackage |
| URL `/pkgsitex/<module>@<tag>` | specific git tag (local mode reads from module cache; the prod worker mode has full tag history) |
| **Show unexported** button | next to the Index header — toggles visibility of private declarations / sidebar / index links; state persists in localStorage across pages |
| **Show internal directories** button | upstream feature — toggles `internal/` subdirectory display. Auto-enabled when `data-local=true` |
| ` `code` ` / `**bold**` / ` ```mermaid ` | godoc comments render these markdown forms (the fork's dochtml ext) |
| **View Source** | a link next to each declaration on the package detail page — jumps to the locally mounted source file (no GitHub access required) |

## Patches (what the fork changes)

- **`-base-path`**: URL subpath prefix — all mux patterns / template helpers / godoc cross-references / view source links are auto-prefixed
- **`-show-unexported`**: makes `internal/fetch/load.go` keep unexported `FuncDecl`s during AST processing, and `doc.NewFromFiles` uses `doc.AllDecls`
- **godoc markdown ext** (`internal/godoc/dochtml/internal/render/markdown_ext.go`): post-processes HTML to recognize inline code / bold / mermaid fences
- **mermaid client lazy-load** (`static/frontend/frontend.tmpl`): dynamically imports `mermaid@10` only when a page contains `code.language-mermaid`
- **unexported toggle** (`static/frontend/unit/main/main.ts`): client-side hide + button + localStorage
- **view source local file mux** + base path prefix fix
- **multi-repo command line**: list multiple module paths in one command
- **trailing-slash redirect-loop fix**: `internal/frontend/details.go` distinguishes "mounted at root" from "the base path itself" in base-path mode
- **prod 4-piece stack**: postgres / athens / worker / frontend `compose.prod.yaml` plus a `cmd/pkgsitex-init` bootstrap container that writes athens netrc + enqueues modules to the worker

## Prod deployment

Prod mode differs from dev — it runs the same 4-component architecture as pkg.go.dev itself:

- **postgres**: caches module index / package docs
- **athens**: GOPROXY cache + private repo fetch via GitHub PAT
- **worker**: async fetches modules into the DB
- **frontend**: renders from the DB (port 8089)

The image is published to docker.io: [`nickwilde18/pkgsitex:fork-main`](https://hub.docker.com/r/nickwilde18/pkgsitex) (multi-arch `linux/amd64` + `linux/arm64`, auto-pushed by GitHub Actions). Users don't need to build — pull the image and run.

```sh
git clone https://github.com/NickWilde18/pkgsitex.git
cd pkgsitex
git checkout fork/main

# 1) configure PAT
cp deploy/prod/.env.example deploy/prod/.env
$EDITOR deploy/prod/.env             # GITHUB_TOKEN=ghp_xxx

# 2) edit the module list (5 sample CUHKSZ internal repos pre-listed)
$EDITOR deploy/prod/config.yaml

# 3) start the stack (image is auto-pulled)
docker compose -f compose.prod.yaml --env-file deploy/prod/.env up -d

# 4) browse
open http://localhost:8089/pkgsitex/
```

Full ops (add/remove modules, periodic cron refresh, troubleshooting): [`deploy/prod/README.md`](deploy/prod/README.md).

Supports multi-version (git tag switching), license-permissive rendering (proprietary too), and config-driven module list.

## Maintenance strategy

- **master always equals upstream master** — never merge `fork/main` into master. GitHub "Sync fork" stays fast-forward, zero conflict
- **`fork/main` is a long-lived patch branch** — all fork changes accumulate here
- **Monthly rebase**: AI runs `git rebase upstream/master` so `fork/main` follows new upstream development. Predicted conflict hot spots:
  - `internal/frontend/server.go` mux list (when upstream adds new routes)
  - `static/**/*.tmpl` / `*.ts` (when upstream changes styles / interactions)
  - other fork-only files (e.g. `markdown_ext.go`, `base-path/base-path.ts`) almost never conflict (fork-exclusive)

Patch history: PR [#2](https://github.com/NickWilde18/pkgsitex/pull/2).

## Troubleshooting

| Symptom | Diagnosis |
|---|---|
| `go run` fails to fetch deps | use a closer GOPROXY: `GOPROXY=https://goproxy.cn,direct go run ...` |
| `/pkgsitex/...` static asset 404 | you edited `static/`/templates without rebuilding: run `go run ./devtools/cmd/static` to regenerate the bundle |
| module resolution timeout | point GOPROXY at the company Athens: `export GOPROXY=https://athens.corp.com,direct` |
| mermaid doesn't render | check the browser console for `mermaid load failed` — your network blocks jsdelivr CDN; vendor mermaid locally (`third_party/mermaid/` pending) |
| Sidebar / Index has no "Show unexported" button | localStorage may have an old toggle key (`gogodocs:showUnexported`); click the button once to reset to the new key |

---

## Upstream pkgsite

A downstream fork has to follow upstream development — keeping the upstream README content below for cross-checking features / locating differences during upgrades.

# golang.org/x/pkgsite

This repository hosts the source code of the [pkg.go.dev](https://pkg.go.dev) website,
and [`pkgsite`](https://pkg.go.dev/golang.org/x/pkgsite/cmd/pkgsite), a documentation
server program.

[![Go Reference](https://pkg.go.dev/badge/golang.org/x/pkgsite.svg)](https://pkg.go.dev/golang.org/x/pkgsite)

Full upstream README: [golang/pkgsite README](https://github.com/golang/pkgsite#readme).

## Issues

Fork issues (base-path / markdown ext / etc): [NickWilde18/pkgsitex/issues](https://github.com/NickWilde18/pkgsitex/issues).

Upstream pkgsite issues go to [`golang/go`](https://golang.org/issues), prefixed `x/pkgsite:` — see the upstream README.

## Contributing

Fork-internal PRs target the `fork/main` branch (not master).

Upstream pkgsite contribution flow (Gerrit code review): [Contribution Guide](https://golang.org/doc/contribute.html) + [upstream README contributing section](https://github.com/golang/pkgsite#contributing).

## License

Unless otherwise noted, the source files are distributed under the BSD-style license found in the LICENSE file.
