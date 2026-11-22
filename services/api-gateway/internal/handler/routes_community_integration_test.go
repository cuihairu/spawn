package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestRoutes_NotificationsProxy(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 列表：Bearer 透传，query 分页透传。
	got := call(t, http.MethodGet, base+"/api/v1/notifications?limit=20&offset=0", "Bearer gw-token", "")
	if got.status != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", got.status, got.body)
	}
	if list := list(t, got.json, "notifications"); len(list) != 1 {
		t.Fatalf("notifications = %#v", list)
	}
	sent := stubs.community.last(t)
	if sent.method != http.MethodGet || sent.path != "/api/v1/notifications" || sent.query != "limit=20&offset=0" {
		t.Fatalf("upstream request = %s %s?%s", sent.method, sent.path, sent.query)
	}

	// 未读数。
	got = call(t, http.MethodGet, base+"/api/v1/notifications/unread-count", "Bearer gw-token", "")
	if got.status != http.StatusOK || got.json["count"] != float64(3) {
		t.Fatalf("unread-count status = %d body = %#v", got.status, got.json)
	}

	// 全部已读：POST 透传。
	got = call(t, http.MethodPost, base+"/api/v1/notifications/read-all", "Bearer gw-token", "")
	if got.status != http.StatusOK || got.json["code"] != float64(0) {
		t.Fatalf("read-all status = %d body = %#v", got.status, got.json)
	}
	sent = stubs.community.last(t)
	if sent.method != http.MethodPost || sent.path != "/api/v1/notifications/read-all" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
}

// --- community 反代：/api/v1/posts/search（帖子搜索，公开端点） ---

func TestRoutes_PostSearchProxy(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 无 token 公开访问，query 原样透传。
	got := call(t, http.MethodGet, base+"/api/v1/posts/search?q=%E8%B7%91%E5%9B%BE&limit=10&offset=0", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("search status = %d, body = %s", got.status, got.body)
	}
	if list := list(t, got.json, "posts"); len(list) != 1 {
		t.Fatalf("posts = %#v", list)
	}
	sent := stubs.community.last(t)
	if sent.method != http.MethodGet || sent.path != "/api/v1/posts/search" ||
		sent.query != "q=%E8%B7%91%E5%9B%BE&limit=10&offset=0" {
		t.Fatalf("upstream request = %s %s?%s", sent.method, sent.path, sent.query)
	}
}

// --- community 反代：/api/v1/moderation（内容审核） ---

func TestRoutes_ModerationProxy(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 举报：POST 透传，Bearer 透传，body 原样转发。
	got := call(t, http.MethodPost, base+"/api/v1/posts/1/report", "Bearer gw-token",
		`{"reason":"垃圾广告"}`)
	if got.status != http.StatusOK {
		t.Fatalf("report status = %d, body = %s", got.status, got.body)
	}
	if got.json["message"] != "report submitted" {
		t.Fatalf("report envelope = %#v", got.json)
	}
	sent := stubs.community.last(t)
	if sent.method != http.MethodPost || sent.path != "/api/v1/posts/1/report" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
	if sent.auth != "Bearer gw-token" || !strings.Contains(sent.body, `"reason":"垃圾广告"`) {
		t.Fatalf("upstream auth/body = %q / %q", sent.auth, sent.body)
	}

	// 队列查看：query 透传。
	got = call(t, http.MethodGet, base+"/api/v1/moderation/reports?status=pending&limit=20&offset=0", "Bearer gw-token", "")
	if got.status != http.StatusOK {
		t.Fatalf("list reports status = %d, body = %s", got.status, got.body)
	}
	if reports := list(t, got.json, "reports"); len(reports) != 1 {
		t.Fatalf("reports = %#v", reports)
	}
	sent = stubs.community.last(t)
	if sent.method != http.MethodGet || sent.path != "/api/v1/moderation/reports" ||
		sent.query != "status=pending&limit=20&offset=0" {
		t.Fatalf("upstream request = %s %s?%s", sent.method, sent.path, sent.query)
	}

	// 处置：POST 透传。
	got = call(t, http.MethodPost, base+"/api/v1/moderation/reports/1/handle", "Bearer gw-token",
		`{"action":"resolve"}`)
	if got.status != http.StatusOK {
		t.Fatalf("handle status = %d, body = %s", got.status, got.body)
	}
	sent = stubs.community.last(t)
	if sent.method != http.MethodPost || sent.path != "/api/v1/moderation/reports/1/handle" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
}

// --- 路由注册本身 ---

// TestRoutes_RegistrationIsExact 校验网关自有 handler 的路由边界：
// 路径存在但方法不符返回 405，路径不存在返回 404，都不会被某条路由兜住，
// 也不会触达上游，证明没有隐式兜底路由。（反代路由由各 proxy 包单测覆盖。）
