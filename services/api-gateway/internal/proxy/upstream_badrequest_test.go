package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serveHTTP 的请求构造错误分支：真 http 服务器会在协议解析层拒绝
// 非法方法 token，handler 永远收不到；白盒直调注入敌意方法，
// 验证兜底 502（与 TestCopyHeaderDeletesHopByHop 同款直测口径）。
func TestUpstream_ServeHTTPInvalidMethodReturns502(t *testing.T) {
	t.Parallel()

	up, err := NewUpstream("http://127.0.0.1:1", 0)
	if err != nil {
		t.Fatalf("NewUpstream: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/games", nil)
	req.Method = "BAD METHOD" // 含空格，非合法方法 token

	rr := httptest.NewRecorder()
	up.Handler()(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
	if body := rr.Body.String(); !strings.Contains(body, `"code":502`) ||
		!strings.Contains(body, "bad gateway") {
		t.Fatalf("body = %q", body)
	}
}
