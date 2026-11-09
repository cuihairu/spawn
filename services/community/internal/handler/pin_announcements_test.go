package handler

// GP-307 官方身份/置顶/公告流（F09）：
// 管理员发帖 → author_official 服务端落标（客户端不可自报）；玩家发帖非官方；
// 置顶仅审核管理员（玩家 403，管理员 200 且幂等）；公告入口公开读、
// 仅官方帖、置顶优先。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/tappi/tappi/services/community/internal/handler/post"
	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

func TestOfficialPostPinAnnouncements(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, secret := newTestServiceContext(t)
	svcCtx.AdminSet = map[int64]bool{1: true}
	adminToken := signToken(t, secret, 1, "admin")
	playerToken := signToken(t, secret, 2, "player")

	create := svcCtx.Auth(post.CreatePostHandler(svcCtx))
	createPost := func(token, title string) int64 {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/posts",
			bytes.NewBufferString(`{"topic_id":1,"title":"`+title+`","content":"正文"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		create(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("create post %s: expected 200, got %d body=%s", title, rr.Code, rr.Body.String())
		}
		var resp types.PostResp
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal PostResp: %v", err)
		}
		return resp.Post.Id
	}

	// 1. 管理员帖 → 官方落标；玩家帖 → 非官方（官方身份由服务端 IsAdmin
	// 判定，请求体无从自报）
	adminA := createPost(adminToken, "官方公告A")
	playerB := createPost(playerToken, "玩家帖B")
	adminC := createPost(adminToken, "官方公告C")

	postA, err := svcCtx.PostRepo.Get(adminA)
	if err != nil || !postA.AuthorOfficial {
		t.Fatalf("admin post author_official=%v err=%v, want true", postA.AuthorOfficial, err)
	}
	postB, err := svcCtx.PostRepo.Get(playerB)
	if err != nil || postB.AuthorOfficial {
		t.Fatalf("player post author_official=%v err=%v, want false", postB.AuthorOfficial, err)
	}

	// 2. 置顶：玩家 403；管理员 200；重复置顶幂等 200
	pin := svcCtx.Auth(post.PinPostHandler(svcCtx))
	pinReq := func(token string, id int64, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+strconv.FormatInt(id, 10)+"/pin",
			bytes.NewBufferString(body))
		req = pathvar.WithVars(req, map[string]string{"id": strconv.FormatInt(id, 10)})
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		pin(rr, req)
		return rr
	}
	if rr := pinReq(playerToken, playerB, `{"pinned":true}`); rr.Code != http.StatusForbidden {
		t.Fatalf("player pin: expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	if rr := pinReq(adminToken, adminA, `{"pinned":true}`); rr.Code != http.StatusOK {
		t.Fatalf("admin pin: expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if rr := pinReq(adminToken, adminA, `{"pinned":true}`); rr.Code != http.StatusOK {
		t.Fatalf("re-pin: expected 200 (idempotent), got %d body=%s", rr.Code, rr.Body.String())
	}
	postA, err = svcCtx.PostRepo.Get(adminA)
	if err != nil || !postA.IsPinned {
		t.Fatalf("post A is_pinned=%v err=%v, want true", postA.IsPinned, err)
	}

	// 3. 公告入口：公开读（无 token）；仅官方帖；置顶优先
	announcements := post.ListAnnouncementsHandler(svcCtx)
	annReq := httptest.NewRequest(http.MethodGet, "/api/v1/announcements", nil)
	annRR := httptest.NewRecorder()
	announcements(annRR, annReq)
	if annRR.Code != http.StatusOK {
		t.Fatalf("announcements: expected 200, got %d body=%s", annRR.Code, annRR.Body.String())
	}
	var listed types.PostsResp
	if err := json.Unmarshal(annRR.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal PostsResp: %v", err)
	}
	if listed.Total != 2 || len(listed.Posts) != 2 {
		t.Fatalf("announcements total=%d len=%d, want 2/2 (official only)", listed.Total, len(listed.Posts))
	}
	if listed.Posts[0].Id != adminA || listed.Posts[1].Id != adminC {
		t.Fatalf("announcements order: [%d %d], want pinned A=%d first then C=%d",
			listed.Posts[0].Id, listed.Posts[1].Id, adminA, adminC)
	}
	for _, p := range listed.Posts {
		if !p.AuthorOfficial {
			t.Fatalf("announcement post %d not official", p.Id)
		}
	}

	// 4. limit 收口 + 取消置顶后顺序回落（id 倒序：C 前 A 后）
	limitReq := httptest.NewRequest(http.MethodGet, "/api/v1/announcements?limit=1", nil)
	limitRR := httptest.NewRecorder()
	announcements(limitRR, limitReq)
	if limitRR.Code != http.StatusOK {
		t.Fatalf("announcements limit: expected 200, got %d", limitRR.Code)
	}
	var limited types.PostsResp
	if err := json.Unmarshal(limitRR.Body.Bytes(), &limited); err != nil || len(limited.Posts) != 1 || limited.Posts[0].Id != adminA {
		t.Fatalf("limit=1 mismatch: err=%v posts=%#v", err, limited.Posts)
	}
	if rr := pinReq(adminToken, adminA, `{"pinned":false}`); rr.Code != http.StatusOK {
		t.Fatalf("unpin: expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	afterRR := httptest.NewRecorder()
	announcements(afterRR, annReq)
	var after types.PostsResp
	if err := json.Unmarshal(afterRR.Body.Bytes(), &after); err != nil || len(after.Posts) != 2 ||
		after.Posts[0].Id != adminC || after.Posts[1].Id != adminA {
		t.Fatalf("after unpin order: err=%v [%d %d], want C=%d first", err,
			after.Posts[0].Id, after.Posts[1].Id, adminC)
	}
	postA, err = svcCtx.PostRepo.Get(adminA)
	if err != nil || postA.IsPinned {
		t.Fatalf("post A is_pinned=%v err=%v, want false after unpin", postA.IsPinned, err)
	}

	// 5. 非法路径参数：帖子不存在 → 404 信封
	if rr := pinReq(adminToken, 99999, `{"pinned":true}`); rr.Code != http.StatusNotFound {
		t.Fatalf("pin missing post: expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}
