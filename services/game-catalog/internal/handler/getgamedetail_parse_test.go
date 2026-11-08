package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/internal/logic"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// TestGetGameDetailHandler_ParseFailure 请求上下文缺少路由变量 id 时
// httpx.Parse 失败 → ErrorCtx 分支（go-zero 对缺失 path 必填字段报错）。
func TestGetGameDetailHandler_ParseFailure(t *testing.T) {
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)

	var c config.Config
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "games.db")
	// GP-004 起空密钥拒绝启动；解析失败测试仅需一个可用的上下文。
	c.Auth.JWTSecret = "parse-test-secret"
	svcCtx := svc.NewServiceContext(c)

	req := httptest.NewRequest(http.MethodGet, "/games/anything", nil) // 未注入 pathvar
	rr := httptest.NewRecorder()
	GetGameDetailHandler(svcCtx)(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (parse failure), body=%s", rr.Code, rr.Body.String())
	}
}
