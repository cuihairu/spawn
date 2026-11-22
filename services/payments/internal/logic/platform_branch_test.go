package logic

// platform.go 剩余补漏分支：platformRole 四类失败（请求构造失败、目录不可达、
// 非 200、JSON 解析失败）、requirePartnerRole 的 非 partner / partner 未绑定 /
// 绑定成功，以及 catalogServerOpen 的请求失败/不可达/非 200/解析失败/
// 命中 open/命中 closed/无匹配。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tappi/tappi/services/payments/internal/svc"
)

// testSvc 只配 game-catalog BaseURL 的最小上下文（platform.go 仅访问该字段）。
func testSvc(base string) *svc.ServiceContext {
	s := &svc.ServiceContext{}
	s.Config.GameCatalog.BaseURL = base
	return s
}

func TestPlatformRole_ErrorArms(t *testing.T) {
	// 请求构造失败（base 无 scheme）→ 错误
	if _, _, err := platformRole(t.Context(), &svc.ServiceContext{}, "Bearer t"); err == nil {
		t.Fatal("bad base = nil error, want error")
	}
	// 目录不可达（连接拒绝）→ 错误
	if _, _, err := platformRole(t.Context(), testSvc("http://127.0.0.1:1"), "Bearer t"); err == nil {
		t.Fatal("unreachable = nil error, want error")
	}
	// 非 200 → 错误
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(st.Close)
	if _, _, err := platformRole(t.Context(), testSvc(st.URL), "Bearer t"); err == nil {
		t.Fatal("status 500 = nil error, want error")
	}
	// JSON 解析失败 → 错误
	st2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	t.Cleanup(st2.Close)
	if _, _, err := platformRole(t.Context(), testSvc(st2.URL), "Bearer t"); err == nil {
		t.Fatal("bad json = nil error, want error")
	}
}

func TestRequireAdminRole_Forbidden(t *testing.T) {
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"player","partner_id":0}`))
	}))
	t.Cleanup(st.Close)
	if err := requireAdminRole(t.Context(), testSvc(st.URL), "Bearer t"); err == nil {
		t.Fatal("player role = nil error, want 403")
	}
}

func TestRequirePartnerRole_Arms(t *testing.T) {
	// 非 partner → 403
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"player","partner_id":0}`))
	}))
	t.Cleanup(st.Close)
	if _, err := requirePartnerRole(t.Context(), testSvc(st.URL), "Bearer t"); err == nil {
		t.Fatal("player role = nil error, want 403")
	}

	// partner 未绑定（partner_id=0）→ 403
	st2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"partner","partner_id":0}`))
	}))
	t.Cleanup(st2.Close)
	if _, err := requirePartnerRole(t.Context(), testSvc(st2.URL), "Bearer t"); err == nil {
		t.Fatal("partner unbound = nil error, want 403")
	}

	// partner 已绑定 → 放行返回 id
	st3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"partner","partner_id":5}`))
	}))
	t.Cleanup(st3.Close)
	pid, err := requirePartnerRole(t.Context(), testSvc(st3.URL), "Bearer t")
	if err != nil || pid != 5 {
		t.Fatalf("partner bound = (%d,%v), want (5,nil)", pid, err)
	}
}

func TestCatalogServerOpen_Arms(t *testing.T) {
	// 请求构造失败（base 无 scheme）
	if _, err := catalogServerOpen(t.Context(), &svc.ServiceContext{}, "game-a", 1); err == nil {
		t.Fatal("bad base = nil error, want error")
	}
	// 目录不可达
	if _, err := catalogServerOpen(t.Context(), testSvc("http://127.0.0.1:1"), "game-a", 1); err == nil {
		t.Fatal("unreachable = nil error, want error")
	}
	// 非 200
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(st.Close)
	if _, err := catalogServerOpen(t.Context(), testSvc(st.URL), "game-a", 1); err == nil {
		t.Fatal("status 502 = nil error, want error")
	}
	// JSON 解析失败
	st2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`nope`))
	}))
	t.Cleanup(st2.Close)
	if _, err := catalogServerOpen(t.Context(), testSvc(st2.URL), "game-a", 1); err == nil {
		t.Fatal("bad json = nil error, want error")
	}
	// 命中 open / 命中 closed / 无匹配
	items := `{"items":[{"id":1,"status":"open"},{"id":2,"status":"closed"}]}`
	st3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(items))
	}))
	t.Cleanup(st3.Close)
	if open, err := catalogServerOpen(t.Context(), testSvc(st3.URL), "game-a", 1); err != nil || !open {
		t.Fatalf("open match = (%v,%v), want (true,nil)", open, err)
	}
	if open, err := catalogServerOpen(t.Context(), testSvc(st3.URL), "game-a", 2); err != nil || open {
		t.Fatalf("closed match = (%v,%v), want (false,nil)", open, err)
	}
	if open, err := catalogServerOpen(t.Context(), testSvc(st3.URL), "game-a", 9); err != nil || open {
		t.Fatalf("no match = (%v,%v), want (false,nil)", open, err)
	}
}
