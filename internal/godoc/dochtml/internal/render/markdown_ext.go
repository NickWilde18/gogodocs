// Copyright 2026 Yechi Yang. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file of the upstream pkgsite.

package render

import (
	"regexp"
	"strings"

	"github.com/google/safehtml"
	"github.com/google/safehtml/uncheckedconversions"
)

// applyDocMarkdownExt 给 godoc 渲染产物加最小的 markdown 风格 inline 修饰。
//
// 背景：godoc 注释走 go/doc/comment parser，不识别 markdown 的反引号
// inline code / **bold** / fenced code blocks——这些会被原样输出成字面文本。
// 但用户写 doc.go 时这些 markdown 风格已经形成习惯（毕竟 .md 写得多），
// 渲染产物里看到 `` `xxx` `` 和 `**bold**` raw text 体验差。
//
// 这层 post-process 把渲染好的 HTML 二次扫描加修饰：
//   - `` `text` `` → <code>text</code>
//   - **text** → <strong>text</strong>
//   - <pre>```mermaid\n...```</pre> → <pre><code class="language-mermaid">...</code></pre>
//     让客户端 mermaid.js 能识别并渲染时序图（frontend.tmpl 里有 lazy-load
//     mermaid CDN 的逻辑，scan 选择器是 code.language-mermaid）
//
// 顺序：先处理 mermaid（在 <pre> 内），再 walkOutsidePre 在 <pre> 块外面
// 做 inline 修饰——避免改到 preformatted code 内的字面字符（用户在缩进
// 4 空格 code block 里写的反引号是真的反引号字面，不是 inline code）。
//
// 设计选型：post-process HTML 比 fork go/doc/comment parser 干净——保留
// godoc 原生的 [Symbol] cross-reference / heading-id / 链接提取等功能不变，
// 只在表层加几条 inline 转换。代价是 regex on HTML 不优雅，但 godoc 渲染
// 的 HTML 结构有限制，<pre> 边界清晰，实测没遇到 corner case。
func applyDocMarkdownExt(h safehtml.HTML) safehtml.HTML {
	s := h.String()
	s = mermaidPreRe.ReplaceAllString(s, `<pre><code class="language-mermaid">$1</code></pre>`)
	s = walkOutsidePre(s, func(seg string) string {
		// 顺序：bold 先于 backtick——`**code**` 先匹配粗体再匹配反引号；
		// 否则反引号匹配会把 ** 包进 code 里破坏。
		seg = boldRe.ReplaceAllString(seg, "<strong>$1</strong>")
		seg = backtickRe.ReplaceAllString(seg, "<code>$1</code>")
		return seg
	})
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(s)
}

var (
	// 反引号 inline code——不跨行（避免 multi-paragraph 误捕、避免与 ``` 围栏
	// 冲突），允许内容含其他字符但不能再含反引号。
	backtickRe = regexp.MustCompile("`([^`\n]+)`")

	// **bold**——不跨行；内容不能含 *，避免吃掉相邻 ** 标记。
	boldRe = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)

	// <pre> 内首行 ```mermaid 起头的围栏块——godoc 把 fenced 当 raw text 渲染
	// 进 <pre>，我们补上 <code class="language-mermaid">。`(?s)` 让 . 匹配换行。
	// 容许尾部 ``` 缺失或带空白（godoc preformatted 块包内时尾部 ``` 可能被
	// 当下一个段落渲染分裂——简化匹配到第一个 </pre> 闭合）。
	mermaidPreRe = regexp.MustCompile("(?s)<pre>(?:\\s*\n)*```mermaid\\s*\n(.+?)(?:\\s*```\\s*)?</pre>")
)

// walkOutsidePre 把 html 切成 <pre>...</pre> 内 / 外两类 segment，对外面 segment
// 调用 fn 转换，<pre> 内原样保留——godoc 把 markdown 4 空格缩进 code block
// 渲染成 <pre>，这里面字符按字面，不该被 inline 替换误改。
func walkOutsidePre(html string, fn func(string) string) string {
	var out strings.Builder
	for {
		idx := strings.Index(html, "<pre>")
		if idx < 0 {
			out.WriteString(fn(html))
			return out.String()
		}
		out.WriteString(fn(html[:idx]))
		end := strings.Index(html[idx:], "</pre>")
		if end < 0 {
			// 残缺 HTML——直接保留剩余部分不再处理
			out.WriteString(html[idx:])
			return out.String()
		}
		end += len("</pre>")
		out.WriteString(html[idx : idx+end])
		html = html[idx+end:]
	}
}
