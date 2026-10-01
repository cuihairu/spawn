package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tappi/tappi/services/community/internal/handler/follow"
	"github.com/tappi/tappi/services/community/internal/handler/topic"
	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

// TestHandlerErrBranches_LogicErrorsReachErrorCtx 请求上下文缺少 user_id 时
// 逻辑层直接 401，handler 的 err 分支（httpx.ErrorCtx）被触达。
// 恒返回 nil error 的信封类 handler（get_topics/get_posts/get_hot_posts）
// 该分支不可达，不在测试范围（见 §5 台账）。
func TestHandlerErrBranches_LogicErrorsReachErrorCtx(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, _ := newTestServiceContext(t)

	// create topic：无鉴权上下文 → 401
	createTopic := topic.CreateTopicHandler(svcCtx)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics", bytes.NewBufferString(`{"name":"demo","description":"desc"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	createTopic(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("create topic: expected 401, got %d body=%s", rr.Code, rr.Body.String())
	}

	// unfollow user：无鉴权上下文 → 401
	unfollow := follow.UnfollowUserHandler(svcCtx)
	req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/users/2/follow", nil)
	req2 = pathvar.WithVars(req2, map[string]string{"user_id": "2"})
	rr2 := httptest.NewRecorder()
	unfollow(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("unfollow user: expected 401, got %d body=%s", rr2.Code, rr2.Body.String())
	}

	// get following topics：无鉴权上下文 → 401
	listFollowing := topic.GetFollowingTopicsHandler(svcCtx)
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/topics/following", nil)
	rr3 := httptest.NewRecorder()
	listFollowing(rr3, req3)
	if rr3.Code != http.StatusUnauthorized {
		t.Fatalf("get following topics: expected 401, got %d body=%s", rr3.Code, rr3.Body.String())
	}
}
