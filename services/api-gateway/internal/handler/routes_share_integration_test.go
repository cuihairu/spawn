package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestRoutes_ShareLink(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/s/p/1", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("share link status = %d, body = %s", got.status, got.body)
	}
	for _, want := range []string{
		"<!DOCTYPE html>", "分享测试帖", "og:title", "og:type", "og:url",
		"spawn:///post/1", "/community/posts/1", "在 spawn 中打开", "在浏览器中查看",
		// html/template 不做 Sprintf 转义：%% 会原样渲染成非法 CSS，锁死这个坑
		"width: 88%",
	} {
		if !strings.Contains(got.body, want) {
			t.Fatalf("share card 缺 %q, body = %s", want, got.body)
		}
	}
	if !strings.Contains(got.body, "这是一段用于分享卡摘要的正文") {
		t.Fatalf("share card 缺正文摘要, body = %s", got.body)
	}

	// 上游契约：只 GET /api/v1/posts/:id，不携带任何 query。
	sent := stubs.community.last(t)
	if sent.method != http.MethodGet || sent.path != "/api/v1/posts/1" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
	if sent.query != "" {
		t.Fatalf("upstream query = %q, want empty", sent.query)
	}
}

func TestRoutes_ShareLinkFallback(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 帖子不存在 → 通用卡片兜底，但深链入口保持可用。
	got := call(t, http.MethodGet, base+"/s/p/999", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("fallback status = %d, body = %s", got.status, got.body)
	}
	if !strings.Contains(got.body, "在 spawn 里看看这篇帖子") {
		t.Fatalf("fallback title 缺失, body = %s", got.body)
	}
	if !strings.Contains(got.body, "spawn:///post/999") || !strings.Contains(got.body, "/community/posts/999") {
		t.Fatalf("fallback 深链缺失, body = %s", got.body)
	}
}

func TestRoutes_ShareLinkBadID(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 路径参数非法 → 400，且不得打到上游。
	got := call(t, http.MethodGet, base+"/s/p/abc", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", got.status, got.body)
	}
	if n := stubs.community.count(t); n != 0 {
		t.Fatalf("上游被调用 %d 次，parse 失败时不应有调用", n)
	}
}

// --- GET /home/feed（BFF 第二阶段：列表聚合 + 字段裁剪） ---
