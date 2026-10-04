package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/community/internal/handler/follow"
	"github.com/tappi/tappi/services/community/internal/handler/post"
	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

// M3 新增端点的错误分支（与 errorbranch_test 同口径直调 handler）：
// 无 user_id 上下文 → logic 的 UserFromContext 401 → handler ErrorCtx；
// 分页参数非数字 → httpx.Parse 失败 → handler 解析错误分支。
func TestM3Handlers_UnauthorizedReachErrorCtx(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, _ := newTestServiceContext(t)

	cases := []struct {
		name   string
		method string
		path   string
		vars   map[string]string
		run    func(w http.ResponseWriter, r *http.Request)
	}{
		{
			"get following users", http.MethodGet, "/api/v1/users/following", nil,
			func(w http.ResponseWriter, r *http.Request) {
				follow.GetFollowingUsersHandler(svcCtx)(w, r)
			},
		},
		{
			"get followed posts", http.MethodGet, "/api/v1/posts/followed", nil,
			func(w http.ResponseWriter, r *http.Request) {
				post.GetFollowedPostsHandler(svcCtx)(w, r)
			},
		},
		{
			"get liked posts", http.MethodGet, "/api/v1/users/likes", nil,
			func(w http.ResponseWriter, r *http.Request) {
				post.GetLikedPostsHandler(svcCtx)(w, r)
			},
		},
		{
			"like post", http.MethodPost, "/api/v1/posts/1/like", map[string]string{"id": "1"},
			func(w http.ResponseWriter, r *http.Request) {
				post.LikePostHandler(svcCtx)(w, r)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.vars != nil {
				req = pathvar.WithVars(req, tc.vars)
			}
			rr := httptest.NewRecorder()
			tc.run(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (body=%s)", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestM3Handlers_ParseErrorOnBadPagination(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, _ := newTestServiceContext(t)

	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		path    string
	}{
		{"followed posts", post.GetFollowedPostsHandler(svcCtx), "/api/v1/posts/followed?limit=abc"},
		{"liked posts", post.GetLikedPostsHandler(svcCtx), "/api/v1/users/likes?limit=abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			tc.handler(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body=%s)", rr.Code, rr.Body.String())
			}
		})
	}
}

// TestM3Handlers_LikedPostsEmptyNormalizesToSlice 鉴权上下文 + 零点赞：
// ListLikedPosts 返回 nil 切片，logic 归一为空数组而非 null（空态契约）。
func TestM3Handlers_LikedPostsEmptyNormalizesToSlice(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, _ := newTestServiceContext(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/likes", nil)
	req = req.WithContext(context.WithValue(req.Context(), "user_id", int64(7)))
	rr := httptest.NewRecorder()
	post.GetLikedPostsHandler(svcCtx)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"posts":[]`) || !strings.Contains(body, `"total":0`) {
		t.Fatalf("body = %s, want posts [] and total 0", body)
	}
}
