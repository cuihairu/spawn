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

// 列表类请求字段全 optional/default，正常路径 Parse 恒成功；
// query 参数类型不匹配（limit=abc）是唯一可达的 Parse 失败入口，
// 覆盖三个列表 handler 的 ErrorCtx 分支。
func TestListHandlers_MalformedQuery(t *testing.T) {
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)

	var c config.Config
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "games.db")
	svcCtx := svc.NewServiceContext(c)

	cases := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"listgames", ListGamesHandler(svcCtx)},
		{"featured", GetFeaturedGamesHandler(svcCtx)},
		{"recommendations", GetRecommendationsHandler(svcCtx)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/games?limit=abc", nil)
			rr := httptest.NewRecorder()
			tc.handler(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (parse failure), body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}
