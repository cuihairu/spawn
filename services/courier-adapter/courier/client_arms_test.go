package courier

// client.go 剩余补漏分支：NewClient 的 timeout<=0 默认臂、APIError 空 message
// 回退 Code 臂、NewAPIError 构造、Logout（nil payload 非 GET → "{}" body 臂）、
// DeleteAccount 错误臂、sessionCall 的解码失败与缺 accessToken/accountId 臂、
// callPath 的 NewRequest 失败/hc.Do 失败/信封解码失败三臂，以及 support 域
// 五个方法各自的 call 错误与数据解码失败臂（含 query 参数臂与 Items nil 空
// 切片臂）。
//
// 登记不可达（不测）：callPath 的 json.Marshal 失败臂（payload 为固定类型
// 结构体/映射）；io.ReadAll 失败臂（io.LimitReader 对内存 body 恒不报错）。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stubClient 起一个按路径分发返回 (status, body) 的 Courier 存根客户端。
func stubClient(t *testing.T, h func(r *http.Request) (int, string)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, body := h(r)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

func TestNewClient_TimeoutDefault(t *testing.T) {
	// timeout<=0 → 默认 5s
	c, err := NewClient("http://127.0.0.1:1", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if c.hc.Timeout != 5*time.Second {
		t.Fatalf("timeout = %v, want 5s default", c.hc.Timeout)
	}
}

func TestAPIErrorArms(t *testing.T) {
	// 空 message → 回退 Code
	if got := NewAPIError(http.StatusBadRequest, "AUTH_INVALID_CREDENTIALS", "").Error(); got != "AUTH_INVALID_CREDENTIALS" {
		t.Fatalf("empty message Error() = %q, want code", got)
	}
	// 有 message → 优先 message
	if got := NewAPIError(http.StatusInternalServerError, "E", "boom").Error(); got != "boom" {
		t.Fatalf("message Error() = %q, want message", got)
	}
}

func TestClient_SessionCallArms(t *testing.T) {
	ctx := context.Background()
	// 解码失败：data 为数字 → dto 反序列化失败
	c := stubClient(t, func(r *http.Request) (int, string) {
		return http.StatusOK, `{"data":123}`
	})
	if _, err := c.Register(ctx, "g", "prod", "a@b.invalid", "pw"); err == nil || !strings.Contains(err.Error(), "decode register session") {
		t.Fatalf("bad session json = %v, want decode error", err)
	}

	// 缺 accessToken/accountId → 响应缺字段臂
	c2 := stubClient(t, func(r *http.Request) (int, string) {
		return http.StatusOK, `{"data":{}}`
	})
	if _, err := c2.Login(ctx, "g", "prod", "a@b.invalid", "pw"); err == nil || !strings.Contains(err.Error(), "missing accessToken/accountId") {
		t.Fatalf("missing fields = %v, want missing-field error", err)
	}
}

func TestClient_CallFailArms(t *testing.T) {
	ctx := context.Background()
	// NewRequest 失败（非法 URL）
	bad, err := NewClient("://bad", time.Second)
	if err != nil {
		t.Fatalf("new bad client: %v", err)
	}
	if _, err := bad.Register(ctx, "g", "prod", "a@b.invalid", "pw"); err == nil {
		t.Fatal("bad url = nil, want error")
	}

	// hc.Do 失败（已关停端点）
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	dc, _ := NewClient(deadURL, time.Second)
	if _, err := dc.Register(ctx, "g", "prod", "a@b.invalid", "pw"); err == nil {
		t.Fatal("dead server = nil, want error")
	}

	// 信封解码失败：200 但 body 非 JSON
	c := stubClient(t, func(r *http.Request) (int, string) {
		return http.StatusOK, "not-json"
	})
	if err := c.Logout(ctx, "g", "prod", "atk"); err == nil || !strings.Contains(err.Error(), "decode courier") {
		t.Fatalf("bad envelope = %v, want decode envelope error", err)
	}
}

func TestClient_LogoutArms(t *testing.T) {
	ctx := context.Background()
	// Logout 成功（nil payload 非 GET → "{}" body 臂）
	var sawBody string
	c := stubClient(t, func(r *http.Request) (int, string) {
		sawBody = r.Header.Get("X-Courier-Game-Id")
		return http.StatusOK, `{"data":{}}`
	})
	if err := c.Logout(ctx, "cg", "prod", "atk"); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if sawBody != "cg" {
		t.Fatalf("logout game header = %q, want cg", sawBody)
	}
	// Logout 失败（500 错误信封）
	c2 := stubClient(t, func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
	})
	if err := c2.Logout(ctx, "cg", "prod", "atk"); err == nil {
		t.Fatal("logout 500 = nil, want error")
	}
}

func TestClient_DeleteFailArms(t *testing.T) {
	// DeleteAccount 错误臂：call 失败 → return nil, err
	c := stubClient(t, func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
	})
	if _, err := c.DeleteAccount(context.Background(), "cg", "prod", "atk"); err == nil {
		t.Fatal("delete 500 = nil, want error")
	}
}

func TestClient_SupportArms(t *testing.T) {
	ctx := context.Background()

	// CreateTicket：call 错误 + 解码失败 + 成功
	cErr := stubClient(t, func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
	})
	if _, err := cErr.CreateTicket(ctx, "g", "prod", "atk", SupportTicketInput{Title: "t", Body: "b"}); err == nil {
		t.Fatal("create 500 = nil, want error")
	}
	cDec := stubClient(t, func(r *http.Request) (int, string) {
		return http.StatusOK, `{"data":123}`
	})
	if _, err := cDec.CreateTicket(ctx, "g", "prod", "atk", SupportTicketInput{Title: "t", Body: "b"}); err == nil || !strings.Contains(err.Error(), "decode support ticket") {
		t.Fatalf("create bad json = %v, want decode error", err)
	}
	cOK := stubClient(t, func(r *http.Request) (int, string) {
		return http.StatusOK, `{"data":{"id":"tk1","title":"t","status":"OPEN","createdAt":"c","updatedAt":"u"}}`
	})
	if tk, err := cOK.CreateTicket(ctx, "g", "prod", "atk", SupportTicketInput{Title: "t", Body: "b"}); err != nil || tk.ID != "tk1" {
		t.Fatalf("create ok = %+v/%v, want tk1", tk, err)
	}

	// ListTickets：cursor 臂 + call 错误 + 解码失败 + 成功
	cCursor := stubClient(t, func(r *http.Request) (int, string) {
		if r.URL.Query().Get("cursor") != "c1" || r.URL.Query().Get("limit") != "10" {
			return http.StatusBadRequest, `{"error":{"code":"BAD_QUERY","message":"q"}}`
		}
		return http.StatusOK, `{"data":{"items":[],"nextCursor":""}}`
	})
	if _, err := cCursor.ListTickets(ctx, "g", "prod", "atk", "10", "c1"); err != nil {
		t.Fatalf("list with cursor: %v", err)
	}
	if _, err := cErr.ListTickets(ctx, "g", "prod", "atk", "", ""); err == nil {
		t.Fatal("list 500 = nil, want error")
	}
	if _, err := cDec.ListTickets(ctx, "g", "prod", "atk", "", ""); err == nil || !strings.Contains(err.Error(), "decode support ticket page") {
		t.Fatalf("list bad json = %v, want decode error", err)
	}
	page, err := cOK.ListTickets(ctx, "g", "prod", "atk", "", "")
	if err != nil || page == nil || len(page.Items) != 0 {
		t.Fatalf("list ok = %+v/%v, want empty page", page, err)
	}

	// TicketDetail：call 错误 + 解码失败 + 成功
	if _, err := cErr.TicketDetail(ctx, "g", "prod", "atk", "tk1"); err == nil {
		t.Fatal("detail 500 = nil, want error")
	}
	if _, err := cDec.TicketDetail(ctx, "g", "prod", "atk", "tk1"); err == nil || !strings.Contains(err.Error(), "decode support ticket detail") {
		t.Fatalf("detail bad json = %v, want decode error", err)
	}
	if _, err := cOK.TicketDetail(ctx, "g", "prod", "atk", "tk1"); err != nil {
		t.Fatalf("detail ok: %v", err)
	}

	// AppendTicketMessage：call 错误 + 解码失败 + 成功
	if _, err := cErr.AppendTicketMessage(ctx, "g", "prod", "atk", "tk1", "hi"); err == nil {
		t.Fatal("append 500 = nil, want error")
	}
	if _, err := cDec.AppendTicketMessage(ctx, "g", "prod", "atk", "tk1", "hi"); err == nil || !strings.Contains(err.Error(), "decode support message") {
		t.Fatalf("append bad json = %v, want decode error", err)
	}
	if _, err := cOK.AppendTicketMessage(ctx, "g", "prod", "atk", "tk1", "hi"); err != nil {
		t.Fatalf("append ok: %v", err)
	}

	// SearchFAQ：limit 臂 + call 错误 + 解码失败 + Items nil 空切片臂 + 成功
	cLimit := stubClient(t, func(r *http.Request) (int, string) {
		q := r.URL.Query()
		if q.Get("keyword") != "refund" || q.Get("limit") != "5" {
			return http.StatusBadRequest, `{"error":{"code":"BAD_QUERY","message":"q"}}`
		}
		return http.StatusOK, `{"data":{"items":[{"id":"f1","question":"q","answer":"a","keywords":["refund"]}]}}`
	})
	if items, err := cLimit.SearchFAQ(ctx, "g", "prod", "atk", "refund", "5"); err != nil || len(items) != 1 || items[0].ID != "f1" {
		t.Fatalf("faq with limit = %+v/%v, want f1", items, err)
	}
	if _, err := cErr.SearchFAQ(ctx, "g", "prod", "atk", "", ""); err == nil {
		t.Fatal("faq 500 = nil, want error")
	}
	if _, err := cDec.SearchFAQ(ctx, "g", "prod", "atk", "", ""); err == nil || !strings.Contains(err.Error(), "decode support faq") {
		t.Fatalf("faq bad json = %v, want decode error", err)
	}
	cNil := stubClient(t, func(r *http.Request) (int, string) {
		return http.StatusOK, `{"data":{}}`
	})
	if items, err := cNil.SearchFAQ(ctx, "g", "prod", "atk", "", ""); err != nil || len(items) != 0 || items == nil {
		t.Fatalf("faq nil items = %+v/%v, want empty non-nil slice", items, err)
	}
}
