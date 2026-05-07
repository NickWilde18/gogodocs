// Copyright 2026 Yechi Yang. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file of the upstream pkgsite.

// Command pkgsitex-init 是 prod stack 的 init container：
//
//  1. 从 /etc/pkgsitex/config.yaml 读 GitHub PAT + 要监控的 module 列表
//  2. 写 /etc/athens/.netrc 让 athens 内部 git 操作走 PAT 认证拉私有 repo
//  3. 等 worker http 就绪
//  4. 对每个 module：可选 git ls-remote --tags 拿所有版本（multi-version 模式）
//     + POST worker /fetch/{module}/@v/{version} 触发 worker 异步入 DB
//
// 进程跑完即退出（Kubernetes init container 模式 / docker compose 一次性 service）。
// 周期 refresh 由独立 cron container（同一镜像，--mode=refresh）做——首次启动
// 拉 latest + 全 tag，后续 cron 拉新增 tag。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 是 prod stack 的配置文件 schema，对应 /etc/pkgsitex/config.yaml。
//
// 用户视角只需关心 github.token + modules——其他字段在 docker-compose 模板里
// 已固定指向同 stack 内部 service 名，不需要改。
type Config struct {
	GitHub struct {
		// Token 是 classic GitHub PAT（repo scope full）——athens 内部 git
		// clone 用它拉私有 repo。明文存入 .netrc，因此 config.yaml + .netrc
		// 都需 600 权限。生产环境通过 env var GITHUB_TOKEN 注入更佳，配置里
		// 写 ${GITHUB_TOKEN} 启动时 expand。
		Token string `yaml:"token"`
	} `yaml:"github"`

	// Modules：prod 启动时 enqueue 给 worker 的 module 列表。每条 path 是
	// Go module path（如 github.com/CUHKSZ-ITSO-Dev/Chat 或 monorepo 子模块
	// github.com/CUHKSZ-ITSO-Dev/UniAuth/uniauth-gf）。
	Modules []ModuleSpec `yaml:"modules"`

	// Worker 内部 service URL；docker-compose 默认 "http://worker:8000"。
	Worker struct {
		URL string `yaml:"url"`
	} `yaml:"worker"`

	// Athens 配置：netrc 输出位置，让 athens 内部 git 拿到 PAT。
	Athens struct {
		NetrcPath string `yaml:"netrcPath"`
	} `yaml:"athens"`
}

// ModuleSpec 一个待索引 module 的配置。
type ModuleSpec struct {
	// Path 是 Go module path（pkg.go.dev URL 的 module 段）
	Path string `yaml:"path"`

	// Versions 控制初始化时 enqueue 哪些版本：
	//   - "latest"（默认）：只 enqueue @latest，worker resolve 成 default branch
	//   - "all-tags"：git ls-remote --tags 拿所有 tag + enqueue 每个 + 还有
	//     latest（fork 内网部署常用——团队希望看每个 release 的文档）
	//
	// 不在配置里枚举具体版本字符串——上游 pkgsite 已经支持任意 module
	// version URL 直接访问会触发 worker 按需 fetch；这里只是启动预热。
	Versions string `yaml:"versions"`

	// RepoURL 可选——module path 跟 git repo URL 不一致时用（如 monorepo
	// 子模块 github.com/CUHKSZ-ITSO-Dev/UniAuth/uniauth-gf 的 repo URL 是
	// github.com/CUHKSZ-ITSO-Dev/UniAuth）。默认从 module path 推断：
	// 取 github.com/<owner>/<repo> 前三段。
	RepoURL string `yaml:"repoUrl"`
}

func main() {
	configPath := flag.String("config", "/etc/pkgsitex/config.yaml", "path to pkgsitex config yaml")
	mode := flag.String("mode", "init",
		"netrc-only = 只写 netrc 退出（在 athens 启动前跑，避免 athens 等 worker 等 athens 循环依赖）；"+
			"init = 等 worker 就绪 + enqueue 全 module（首次启动）；"+
			"refresh = 跳过 netrc / wait，仅重 ls-remote tags + enqueue 新版本（周期 cron）")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config %s: %v", *configPath, err)
	}
	cfg.GitHub.Token = os.ExpandEnv(cfg.GitHub.Token)
	if cfg.GitHub.Token == "" {
		log.Fatal("github.token 为空（环境变量 GITHUB_TOKEN 未设置 / config 没填）")
	}

	switch *mode {
	case "netrc-only":
		if err := writeNetrc(cfg.Athens.NetrcPath, cfg.GitHub.Token); err != nil {
			log.Fatalf("写 netrc 失败: %v", err)
		}
		log.Printf("[netrc-only] netrc 写入 %s", cfg.Athens.NetrcPath)

	case "init":
		if err := writeNetrc(cfg.Athens.NetrcPath, cfg.GitHub.Token); err != nil {
			log.Fatalf("写 netrc 失败: %v", err)
		}
		log.Printf("[init] netrc 写入 %s", cfg.Athens.NetrcPath)

		if err := waitWorkerReady(ctx, cfg.Worker.URL); err != nil {
			log.Fatalf("等 worker 就绪超时: %v", err)
		}
		log.Printf("[init] worker %s 就绪", cfg.Worker.URL)

		enqueueAll(ctx, cfg)
		log.Printf("[init] 完成")

	case "refresh":
		// refresh 模式跳过 netrc / worker wait（cron 跑时 stack 已稳定运行）
		enqueueAll(ctx, cfg)
		log.Printf("[refresh] 完成")

	default:
		log.Fatalf("未知 mode=%q（支持 netrc-only / init / refresh）", *mode)
	}
}

func loadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("yaml unmarshal: %w", err)
	}
	if cfg.Worker.URL == "" {
		cfg.Worker.URL = "http://worker:8000"
	}
	if cfg.Athens.NetrcPath == "" {
		cfg.Athens.NetrcPath = "/etc/athens/.netrc"
	}
	return &cfg, nil
}

// writeNetrc 输出 athens 用的 .netrc 让 git 走 PAT 认证。
//
// 文件格式（git 标准）：
//
//	machine github.com
//	login x-access-token
//	password <PAT>
//
// "x-access-token" 是 GitHub 的 PAT username 占位（HTTPS Basic Auth 协议要求
// username 非空，PAT 走 password 字段）。
//
// 同时确保 dir 存在 + 权限 600（含 PAT 明文）。
func writeNetrc(path, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	content := fmt.Sprintf("machine github.com\nlogin x-access-token\npassword %s\n", token)
	return os.WriteFile(path, []byte(content), 0o600)
}

// waitWorkerReady 轮询 worker /healthz 端点直到就绪或 ctx 超时。
//
// 间隔 2 秒；总超时由 caller ctx 控制。docker-compose 的 healthcheck 应该
// 已让 worker 在 init 启动前就 ready，但这层兜底防容器启动顺序 race。
func waitWorkerReady(ctx context.Context, workerURL string) error {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, workerURL+"/healthz", nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 400 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// enqueueAll 遍历 cfg.Modules，对每个 module 触发 worker 拉相应 version。
//
// versions=all-tags 时先 git ls-remote --tags 拿全 tag 再 enqueue 每个；否则只
// enqueue @latest（worker 自己 resolve）。错误只 log 不 fatal——单 module 失败
// 不该阻塞其他。
func enqueueAll(ctx context.Context, cfg *Config) {
	for _, mod := range cfg.Modules {
		versions := []string{"latest"}
		if mod.Versions == "all-tags" {
			tags, err := lsRemoteTags(ctx, mod, cfg.GitHub.Token)
			if err != nil {
				log.Printf("ls-remote %s: %v（跳过，仅 enqueue @latest）", mod.Path, err)
			} else {
				versions = append(versions, tags...)
			}
		}
		for _, v := range versions {
			if err := enqueueOne(ctx, cfg.Worker.URL, mod.Path, v); err != nil {
				log.Printf("enqueue %s@%s: %v", mod.Path, v, err)
				continue
			}
			log.Printf("enqueue %s@%s 成功", mod.Path, v)
		}
	}
}

// lsRemoteTags 跑 git ls-remote --tags 拿 module 对应 repo 的所有 git tag。
//
// 用 https://x-access-token:<PAT>@github.com/<repo> 形式让 git 走 PAT 认证。
// PAT 出现在 cmd 行参数里——子进程不会 leak（exec.Command 不写 shell history），
// 但 ps 临时可见——内网容器内部环境可接受。
//
// repoURL 推断：默认取 module path 前三段（github.com/<owner>/<repo>），
// monorepo 子模块走显式 RepoURL 字段。
//
// 返回值过滤：peeled tag refs（refs/tags/v1.0^{}）跳掉，pkgsite 只认 vX.Y.Z
// 形式的 SemVer tag——非 SemVer tag 也 enqueue 但 worker 大概率会拒（log 警告
// 但不 fatal，让 caller 看到详细错误码）。
func lsRemoteTags(ctx context.Context, mod ModuleSpec, token string) ([]string, error) {
	repoURL := mod.RepoURL
	if repoURL == "" {
		// 从 module path 推断：取前三段
		parts := strings.Split(mod.Path, "/")
		if len(parts) < 3 || parts[0] != "github.com" {
			return nil, fmt.Errorf("无法推断 repo URL（仅支持 github.com/owner/repo... 形式，得到 %q）", mod.Path)
		}
		repoURL = strings.Join(parts[:3], "/")
	}

	authedURL := fmt.Sprintf("https://x-access-token:%s@%s", url.PathEscape(token), repoURL)
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", authedURL)
	out, err := cmd.Output()
	if err != nil {
		// 不 leak token 到错误信息：只显示 repoURL 不含 token
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("git 二进制不在 PATH（请检查 init container image）")
		}
		return nil, fmt.Errorf("git ls-remote %s 失败: %w", repoURL, err)
	}

	var tags []string
	for _, line := range strings.Split(string(out), "\n") {
		idx := strings.Index(line, "refs/tags/")
		if idx < 0 {
			continue
		}
		tag := line[idx+len("refs/tags/"):]
		// peeled tag refs（如 "refs/tags/v1.0.0^{}"）跳过——它们指向 commit
		// 而不是 tag object 本身，重复出现
		if strings.HasSuffix(tag, "^{}") {
			continue
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// enqueueOne 调 worker /fetch/<module>/@v/<version> 端点触发异步 fetch。
//
// 该端点是 pkgsite 上游 worker 内置——worker.handleFetch 处理：先看 DB 是否
// 已有此版本数据，没有就 enqueue 进 pgqueue / inmem queue 走 ProxyClient
// 拉模块（我们 stack 里 ProxyClient 指 athens，athens 走 git + PAT 拉私有
// repo）。
//
// HTTP 200 = 已 enqueue（不代表 fetch 成功，那是异步事），4xx/5xx 才是真
// 错误（典型：module path 错 / version 格式不合 SemVer）。
func enqueueOne(ctx context.Context, workerURL, modulePath, version string) error {
	u := fmt.Sprintf("%s/fetch/%s/@v/%s", workerURL, modulePath, version)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
