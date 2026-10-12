package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tappi/tappi/services/community/internal/config"
	comment "github.com/tappi/tappi/services/community/internal/handler/comment"
	message "github.com/tappi/tappi/services/community/internal/handler/message"
	"github.com/tappi/tappi/services/community/internal/handler/moderation"
	notification "github.com/tappi/tappi/services/community/internal/handler/notification"
	"github.com/tappi/tappi/services/community/internal/handler/post"
	"github.com/tappi/tappi/services/community/internal/handler/topic"
	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
	"github.com/tappi/tappi/services/community/utils"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

func newTestServiceContext(t *testing.T) (*svc.ServiceContext, string) {
	t.Helper()

	secret := "test-jwt-secret"

	var c config.Config
	c.Auth.JWTSecret = secret
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "community.db")

	return svc.NewServiceContext(c), secret
}

func signToken(t *testing.T, secret string, userId int64, username string) string {
	t.Helper()

	claims := utils.JWTClaims{
		UserId:   userId,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return signed
}

func TestProtectedHandlers_RequireValidBearerToken(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, secret := newTestServiceContext(t)
	handler := svcCtx.Auth(topic.CreateTopicHandler(svcCtx))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics", bytes.NewBufferString(`{"name":"demo","description":"desc"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr.Code, rr.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/topics", bytes.NewBufferString(`{"name":"demo","description":"desc"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Token abc")
	rr2 := httptest.NewRecorder()
	handler(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr2.Code, rr2.Body.String())
	}

	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/topics", bytes.NewBufferString(`{"name":"demo","description":"desc"}`))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "Bearer bad.token.value")
	rr3 := httptest.NewRecorder()
	handler(rr3, req3)
	if rr3.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr3.Code, rr3.Body.String())
	}

	token := signToken(t, secret, 100, "tester")
	req4 := httptest.NewRequest(http.MethodPost, "/api/v1/topics", bytes.NewBufferString(`{"name":"demo","description":"desc"}`))
	req4.Header.Set("Content-Type", "application/json")
	req4.Header.Set("Authorization", "Bearer "+token)
	rr4 := httptest.NewRecorder()
	handler(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr4.Code, rr4.Body.String())
	}

	var created types.TopicResp
	if err := json.Unmarshal(rr4.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal TopicResp: %v", err)
	}
	if created.Topic.Id <= 0 || created.Topic.Name != "demo" {
		t.Fatalf("unexpected topic resp: %#v", created)
	}
}

func TestTopicFollowFlow_UpdatesCountsAndFollowingList(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, secret := newTestServiceContext(t)
	token := signToken(t, secret, 777, "tester")

	createTopic := svcCtx.Auth(topic.CreateTopicHandler(svcCtx))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics", bytes.NewBufferString(`{"name":"follows","description":"desc"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	createTopic(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create topic: expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var created types.TopicResp
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal TopicResp: %v", err)
	}

	follow := svcCtx.Auth(topic.FollowTopicHandler(svcCtx))
	followReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/"+strconv.FormatInt(created.Topic.Id, 10)+"/follow", nil)
	followReq = pathvar.WithVars(followReq, map[string]string{"topic_id": strconv.FormatInt(created.Topic.Id, 10)})
	followReq.Header.Set("Authorization", "Bearer "+token)
	followRR := httptest.NewRecorder()
	follow(followRR, followReq)
	if followRR.Code != http.StatusOK {
		t.Fatalf("follow: expected 200, got %d body=%s", followRR.Code, followRR.Body.String())
	}

	got, err := svcCtx.TopicRepo.Get(created.Topic.Id)
	if err != nil {
		t.Fatalf("TopicRepo.Get: %v", err)
	}
	if got.FollowerCount != 1 {
		t.Fatalf("expected follower_count=1, got %d", got.FollowerCount)
	}

	listFollowing := svcCtx.Auth(topic.GetFollowingTopicsHandler(svcCtx))
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/topics/following", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRR := httptest.NewRecorder()
	listFollowing(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("following: expected 200, got %d body=%s", listRR.Code, listRR.Body.String())
	}

	var following types.FollowingResp
	if err := json.Unmarshal(listRR.Body.Bytes(), &following); err != nil {
		t.Fatalf("unmarshal FollowingResp: %v", err)
	}
	found := false
	for _, tpc := range following.Topics {
		if tpc.Id == created.Topic.Id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected created topic to appear in following list: %#v", following.Topics)
	}
}

func TestCreatePost_IncrementsTopicPostCount(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, secret := newTestServiceContext(t)
	token := signToken(t, secret, 888, "poster")

	create := svcCtx.Auth(post.CreatePostHandler(svcCtx))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(`{"topic_id":1,"title":"hi","content":"there"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	create(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create post: expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	topic1, err := svcCtx.TopicRepo.Get(1)
	if err != nil {
		t.Fatalf("TopicRepo.Get(1): %v", err)
	}
	if topic1.PostCount != 1 {
		t.Fatalf("expected topic post_count to be incremented, got %d", topic1.PostCount)
	}
}

func TestNotificationEndpoints_Flow(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, secret := newTestServiceContext(t)
	authorToken := signToken(t, secret, 7, "作者甲")
	likerToken := signToken(t, secret, 8, "读者乙")

	// 作者发帖
	create := svcCtx.Auth(post.CreatePostHandler(svcCtx))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(`{"topic_id":1,"title":"通知流","content":"正文"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authorToken)
	rr := httptest.NewRecorder()
	create(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create post: expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var created types.PostResp
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal PostResp: %v", err)
	}
	postId := strconv.FormatInt(created.Post.Id, 10)

	// 读者点赞 → 触发作者通知
	like := svcCtx.Auth(post.LikePostHandler(svcCtx))
	likeReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+postId+"/like", nil)
	likeReq = pathvar.WithVars(likeReq, map[string]string{"id": postId})
	likeReq.Header.Set("Authorization", "Bearer "+likerToken)
	likeRR := httptest.NewRecorder()
	like(likeRR, likeReq)
	if likeRR.Code != http.StatusOK {
		t.Fatalf("like: expected 200, got %d body=%s", likeRR.Code, likeRR.Body.String())
	}

	// 无 token 列表 → 401
	list := svcCtx.Auth(notification.ListNotificationsHandler(svcCtx))
	bareReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	bareRR := httptest.NewRecorder()
	list(bareRR, bareReq)
	if bareRR.Code != http.StatusUnauthorized {
		t.Fatalf("list without token: expected 401, got %d", bareRR.Code)
	}

	// 作者列表 → 1 条 like_post
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?limit=20&offset=0", nil)
	listReq.Header.Set("Authorization", "Bearer "+authorToken)
	listRR := httptest.NewRecorder()
	list(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d body=%s", listRR.Code, listRR.Body.String())
	}
	var listed types.NotificationsResp
	if err := json.Unmarshal(listRR.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal NotificationsResp: %v", err)
	}
	if listed.Total != 1 || len(listed.Notifications) != 1 || listed.Notifications[0].Type != "like_post" {
		t.Fatalf("notifications mismatch: %#v", listed)
	}

	// 未读数 → 1
	unread := svcCtx.Auth(notification.GetUnreadCountHandler(svcCtx))
	unreadReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
	unreadReq.Header.Set("Authorization", "Bearer "+authorToken)
	unreadRR := httptest.NewRecorder()
	unread(unreadRR, unreadReq)
	if unreadRR.Code != http.StatusOK {
		t.Fatalf("unread-count: expected 200, got %d body=%s", unreadRR.Code, unreadRR.Body.String())
	}
	var unreadResp types.UnreadCountResp
	if err := json.Unmarshal(unreadRR.Body.Bytes(), &unreadResp); err != nil {
		t.Fatalf("unmarshal UnreadCountResp: %v", err)
	}
	if unreadResp.Count != 1 {
		t.Fatalf("unread count=%d, want 1", unreadResp.Count)
	}

	// 单条已读：like_post 置已读 → 未读数 1→0；幂等重标仍 200；他人/不存在 → 404 信封
	notifId := strconv.FormatInt(listed.Notifications[0].Id, 10)
	markOne := svcCtx.Auth(notification.MarkNotificationReadHandler(svcCtx))
	markOneReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/"+notifId+"/read", nil)
	markOneReq = pathvar.WithVars(markOneReq, map[string]string{"id": notifId})
	markOneReq.Header.Set("Authorization", "Bearer "+authorToken)
	markOneRR := httptest.NewRecorder()
	markOne(markOneRR, markOneReq)
	if markOneRR.Code != http.StatusOK {
		t.Fatalf("mark one read: expected 200, got %d body=%s", markOneRR.Code, markOneRR.Body.String())
	}
	unreadOneRR := httptest.NewRecorder()
	unread(unreadOneRR, unreadReq)
	var unreadRespOne types.UnreadCountResp
	if err := json.Unmarshal(unreadOneRR.Body.Bytes(), &unreadRespOne); err != nil {
		t.Fatalf("unmarshal UnreadCountResp: %v", err)
	}
	if unreadRespOne.Count != 0 {
		t.Fatalf("unread count after mark one=%d, want 0", unreadRespOne.Count)
	}
	// 幂等重标
	markOneRR2 := httptest.NewRecorder()
	markOne(markOneRR2, markOneReq)
	if markOneRR2.Code != http.StatusOK {
		t.Fatalf("re-mark one: expected 200, got %d body=%s", markOneRR2.Code, markOneRR2.Body.String())
	}
	// 非本人（读者乙拿作者的通知 id）→ 404
	markForeignReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/"+notifId+"/read", nil)
	markForeignReq = pathvar.WithVars(markForeignReq, map[string]string{"id": notifId})
	markForeignReq.Header.Set("Authorization", "Bearer "+likerToken)
	markForeignRR := httptest.NewRecorder()
	markOne(markForeignRR, markForeignReq)
	if markForeignRR.Code != http.StatusNotFound {
		t.Fatalf("foreign mark one: expected 404, got %d body=%s", markForeignRR.Code, markForeignRR.Body.String())
	}

	// 全部已读 → 未读数归零
	markRead := svcCtx.Auth(notification.MarkAllReadHandler(svcCtx))
	markReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read-all", nil)
	markReq.Header.Set("Authorization", "Bearer "+authorToken)
	markRR := httptest.NewRecorder()
	markRead(markRR, markReq)
	if markRR.Code != http.StatusOK {
		t.Fatalf("read-all: expected 200, got %d body=%s", markRR.Code, markRR.Body.String())
	}
	unreadReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
	unreadReq2.Header.Set("Authorization", "Bearer "+authorToken)
	unreadRR2 := httptest.NewRecorder()
	unread(unreadRR2, unreadReq2)
	var unreadResp2 types.UnreadCountResp
	if err := json.Unmarshal(unreadRR2.Body.Bytes(), &unreadResp2); err != nil {
		t.Fatalf("unmarshal UnreadCountResp: %v", err)
	}
	if unreadResp2.Count != 0 {
		t.Fatalf("unread count after read-all=%d, want 0", unreadResp2.Count)
	}

	// 列表 Parse 失败分支 → 400 信封
	badListReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?limit=abc", nil)
	badListReq.Header.Set("Authorization", "Bearer "+authorToken)
	badListRR := httptest.NewRecorder()
	list(badListRR, badListReq)
	if badListRR.Code != http.StatusBadRequest {
		t.Fatalf("bad limit: expected 400, got %d body=%s", badListRR.Code, badListRR.Body.String())
	}
}

func TestModerationReportFlow(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, secret := newTestServiceContext(t)
	authorToken := signToken(t, secret, 7, "作者甲")
	readerToken := signToken(t, secret, 8, "读者乙")
	adminToken := signToken(t, secret, 1, "管理员")
	svcCtx.AdminSet = map[int64]bool{1: true}

	// 敏感词帖子 → 400 拒绝
	create := svcCtx.Auth(post.CreatePostHandler(svcCtx))
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(`{"topic_id":1,"title":"t","content":"卖外挂"}`))
	badReq.Header.Set("Content-Type", "application/json")
	badReq.Header.Set("Authorization", "Bearer "+authorToken)
	badRR := httptest.NewRecorder()
	create(badRR, badReq)
	if badRR.Code != http.StatusBadRequest {
		t.Fatalf("blocked word post: expected 400, got %d body=%s", badRR.Code, badRR.Body.String())
	}

	// 正常发帖
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(`{"topic_id":1,"title":"举报流","content":"正文"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authorToken)
	rr := httptest.NewRecorder()
	create(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create post: expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var created types.PostResp
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal PostResp: %v", err)
	}
	postId := strconv.FormatInt(created.Post.Id, 10)

	// 读者举报 → 200
	report := svcCtx.Auth(moderation.ReportPostHandler(svcCtx))
	reportReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+postId+"/report", bytes.NewBufferString(`{"reason":"垃圾广告"}`))
	reportReq.Header.Set("Content-Type", "application/json")
	reportReq = pathvar.WithVars(reportReq, map[string]string{"id": postId})
	reportReq.Header.Set("Authorization", "Bearer "+readerToken)
	reportRR := httptest.NewRecorder()
	report(reportRR, reportReq)
	if reportRR.Code != http.StatusOK {
		t.Fatalf("report: expected 200, got %d body=%s", reportRR.Code, reportRR.Body.String())
	}

	// 非 admin 查看队列 → 403
	listReports := svcCtx.Auth(moderation.ListReportsHandler(svcCtx))
	denyReq := httptest.NewRequest(http.MethodGet, "/api/v1/moderation/reports", nil)
	denyReq.Header.Set("Authorization", "Bearer "+readerToken)
	denyRR := httptest.NewRecorder()
	listReports(denyRR, denyReq)
	if denyRR.Code != http.StatusForbidden {
		t.Fatalf("non-admin list reports: expected 403, got %d body=%s", denyRR.Code, denyRR.Body.String())
	}

	// admin 查看队列 → 1 条 pending
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/moderation/reports?status=pending", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRR := httptest.NewRecorder()
	listReports(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("admin list reports: expected 200, got %d body=%s", listRR.Code, listRR.Body.String())
	}
	var listed types.ReportsResp
	if err := json.Unmarshal(listRR.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal ReportsResp: %v", err)
	}
	if listed.Total != 1 || len(listed.Reports) != 1 || listed.Reports[0].Status != "pending" || listed.Reports[0].TargetId != created.Post.Id {
		t.Fatalf("reports mismatch: %#v", listed)
	}
	reportId := strconv.FormatInt(listed.Reports[0].Id, 10)

	// admin resolve → 帖子软删 + 状态 resolved
	handle := svcCtx.Auth(moderation.HandleReportHandler(svcCtx))
	handleReq := httptest.NewRequest(http.MethodPost, "/api/v1/moderation/reports/"+reportId+"/handle", bytes.NewBufferString(`{"action":"resolve"}`))
	handleReq.Header.Set("Content-Type", "application/json")
	handleReq = pathvar.WithVars(handleReq, map[string]string{"id": reportId})
	handleReq.Header.Set("Authorization", "Bearer "+adminToken)
	handleRR := httptest.NewRecorder()
	handle(handleRR, handleReq)
	if handleRR.Code != http.StatusOK {
		t.Fatalf("resolve: expected 200, got %d body=%s", handleRR.Code, handleRR.Body.String())
	}

	get := post.GetPostHandler(svcCtx)
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+postId, nil)
	getReq = pathvar.WithVars(getReq, map[string]string{"id": postId})
	getRR := httptest.NewRecorder()
	get(getRR, getReq)
	if getRR.Code != http.StatusNotFound {
		t.Fatalf("resolved post: expected 404, got %d body=%s", getRR.Code, getRR.Body.String())
	}

	// 二次处置 → 400
	rehandleReq := httptest.NewRequest(http.MethodPost, "/api/v1/moderation/reports/"+reportId+"/handle", bytes.NewBufferString(`{"action":"dismiss"}`))
	rehandleReq.Header.Set("Content-Type", "application/json")
	rehandleReq = pathvar.WithVars(rehandleReq, map[string]string{"id": reportId})
	rehandleReq.Header.Set("Authorization", "Bearer "+adminToken)
	rehandleRR := httptest.NewRecorder()
	handle(rehandleRR, rehandleReq)
	if rehandleRR.Code != http.StatusBadRequest {
		t.Fatalf("re-handle: expected 400, got %d body=%s", rehandleRR.Code, rehandleRR.Body.String())
	}

	// 举报 Parse 失败分支 → 400 信封
	badReportReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+postId+"/report", bytes.NewBufferString(`{"reason":}`))
	badReportReq.Header.Set("Content-Type", "application/json")
	badReportReq = pathvar.WithVars(badReportReq, map[string]string{"id": postId})
	badReportReq.Header.Set("Authorization", "Bearer "+readerToken)
	badReportRR := httptest.NewRecorder()
	report(badReportRR, badReportReq)
	if badReportRR.Code != http.StatusBadRequest {
		t.Fatalf("bad report body: expected 400, got %d body=%s", badReportRR.Code, badReportRR.Body.String())
	}

	// 举报不存在的帖子 → 404（logic 层 PostRepo.Get 错误）
	ghostReportReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts/9999/report", bytes.NewBufferString(`{"reason":"x"}`))
	ghostReportReq.Header.Set("Content-Type", "application/json")
	ghostReportReq = pathvar.WithVars(ghostReportReq, map[string]string{"id": "9999"})
	ghostReportReq.Header.Set("Authorization", "Bearer "+readerToken)
	ghostReportRR := httptest.NewRecorder()
	report(ghostReportRR, ghostReportReq)
	if ghostReportRR.Code != http.StatusNotFound {
		t.Fatalf("ghost report: expected 404, got %d body=%s", ghostReportRR.Code, ghostReportRR.Body.String())
	}

	// 队列列表 Parse 失败分支 → 400
	badListReq := httptest.NewRequest(http.MethodGet, "/api/v1/moderation/reports?limit=abc", nil)
	badListReq.Header.Set("Authorization", "Bearer "+adminToken)
	badListRR := httptest.NewRecorder()
	listReports(badListRR, badListReq)
	if badListRR.Code != http.StatusBadRequest {
		t.Fatalf("bad list limit: expected 400, got %d body=%s", badListRR.Code, badListRR.Body.String())
	}

	// 处置 Parse 失败分支 → 400
	badHandleReq := httptest.NewRequest(http.MethodPost, "/api/v1/moderation/reports/"+reportId+"/handle", bytes.NewBufferString(`{"action":}`))
	badHandleReq.Header.Set("Content-Type", "application/json")
	badHandleReq = pathvar.WithVars(badHandleReq, map[string]string{"id": reportId})
	badHandleReq.Header.Set("Authorization", "Bearer "+adminToken)
	badHandleRR := httptest.NewRecorder()
	handle(badHandleRR, badHandleReq)
	if badHandleRR.Code != http.StatusBadRequest {
		t.Fatalf("bad handle body: expected 400, got %d body=%s", badHandleRR.Code, badHandleRR.Body.String())
	}
}

// TestCommentEndpoints_Flow 评论三端点真 handler 全链路：
// 匿名列表（公开）→ 匿名发评 401 → 发评/回评（冗余被回复者昵称）→
// 列表总数 → 越权删评 403 → 作者删评 → 计数核对。补齐与 post/topic/
// notification/moderation 同款的 handler 层流测。
func TestCommentEndpoints_Flow(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, secret := newTestServiceContext(t)
	authorToken := signToken(t, secret, 7, "作者甲")
	readerToken := signToken(t, secret, 8, "读者乙")

	// 作者发帖
	create := svcCtx.Auth(post.CreatePostHandler(svcCtx))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(`{"topic_id":1,"title":"评论流","content":"正文"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authorToken)
	rr := httptest.NewRecorder()
	create(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create post: expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var created types.PostResp
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal PostResp: %v", err)
	}
	postId := strconv.FormatInt(created.Post.Id, 10)

	list := comment.ListCommentsHandler(svcCtx)

	// 匿名列表（公开端点）→ 200 空列表
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+postId+"/comments", nil)
	listReq = pathvar.WithVars(listReq, map[string]string{"id": postId})
	listRR := httptest.NewRecorder()
	list(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("anonymous list: expected 200, got %d body=%s", listRR.Code, listRR.Body.String())
	}
	var listed types.CommentsResp
	if err := json.Unmarshal(listRR.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal CommentsResp: %v", err)
	}
	if listed.Total != 0 || len(listed.Comments) != 0 {
		t.Fatalf("initial list = %#v, want empty", listed)
	}

	// 匿名发评 → 401
	postComment := svcCtx.Auth(comment.CreateCommentHandler(svcCtx))
	bareReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+postId+"/comments", bytes.NewBufferString(`{"content":"hi"}`))
	bareReq.Header.Set("Content-Type", "application/json")
	bareReq = pathvar.WithVars(bareReq, map[string]string{"id": postId})
	bareRR := httptest.NewRecorder()
	postComment(bareRR, bareReq)
	if bareRR.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous comment: expected 401, got %d body=%s", bareRR.Code, bareRR.Body.String())
	}

	// 读者发评 → 200
	readerCReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+postId+"/comments", bytes.NewBufferString(`{"content":"沙发"}`))
	readerCReq.Header.Set("Content-Type", "application/json")
	readerCReq = pathvar.WithVars(readerCReq, map[string]string{"id": postId})
	readerCReq.Header.Set("Authorization", "Bearer "+readerToken)
	readerCRR := httptest.NewRecorder()
	postComment(readerCRR, readerCReq)
	if readerCRR.Code != http.StatusOK {
		t.Fatalf("reader comment: expected 200, got %d body=%s", readerCRR.Code, readerCRR.Body.String())
	}
	var readerComment types.CommentResp
	if err := json.Unmarshal(readerCRR.Body.Bytes(), &readerComment); err != nil {
		t.Fatalf("unmarshal CommentResp: %v", err)
	}
	if readerComment.Comment == nil || readerComment.Comment.Id <= 0 || readerComment.Comment.AuthorName != "读者乙" {
		t.Fatalf("reader comment = %#v", readerComment.Comment)
	}

	// 不存在的帖子发评 → 404
	ghostReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts/9999/comments", bytes.NewBufferString(`{"content":"hi"}`))
	ghostReq.Header.Set("Content-Type", "application/json")
	ghostReq = pathvar.WithVars(ghostReq, map[string]string{"id": "9999"})
	ghostReq.Header.Set("Authorization", "Bearer "+readerToken)
	ghostRR := httptest.NewRecorder()
	postComment(ghostRR, ghostReq)
	if ghostRR.Code != http.StatusNotFound {
		t.Fatalf("ghost post comment: expected 404, got %d body=%s", ghostRR.Code, ghostRR.Body.String())
	}

	// 作者发评，读者回复作者评论 → 冗余被回复者昵称
	authorCReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+postId+"/comments", bytes.NewBufferString(`{"content":"欢迎"}`))
	authorCReq.Header.Set("Content-Type", "application/json")
	authorCReq = pathvar.WithVars(authorCReq, map[string]string{"id": postId})
	authorCReq.Header.Set("Authorization", "Bearer "+authorToken)
	authorCRR := httptest.NewRecorder()
	postComment(authorCRR, authorCReq)
	if authorCRR.Code != http.StatusOK {
		t.Fatalf("author comment: expected 200, got %d body=%s", authorCRR.Code, authorCRR.Body.String())
	}
	var authorComment types.CommentResp
	if err := json.Unmarshal(authorCRR.Body.Bytes(), &authorComment); err != nil {
		t.Fatalf("unmarshal author CommentResp: %v", err)
	}
	parentId := strconv.FormatInt(authorComment.Comment.Id, 10)

	replyReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+postId+"/comments", bytes.NewBufferString(`{"content":"回复作者","parent_id":`+parentId+`}`))
	replyReq.Header.Set("Content-Type", "application/json")
	replyReq = pathvar.WithVars(replyReq, map[string]string{"id": postId})
	replyReq.Header.Set("Authorization", "Bearer "+readerToken)
	replyRR := httptest.NewRecorder()
	postComment(replyRR, replyReq)
	if replyRR.Code != http.StatusOK {
		t.Fatalf("reply: expected 200, got %d body=%s", replyRR.Code, replyRR.Body.String())
	}
	var reply types.CommentResp
	if err := json.Unmarshal(replyRR.Body.Bytes(), &reply); err != nil {
		t.Fatalf("unmarshal reply CommentResp: %v", err)
	}
	if reply.Comment.ParentId != authorComment.Comment.Id || reply.Comment.ReplyToAuthorName != "作者甲" {
		t.Fatalf("reply = %#v", reply.Comment)
	}

	// 列表 → 3 条
	listReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+postId+"/comments", nil)
	listReq2 = pathvar.WithVars(listReq2, map[string]string{"id": postId})
	listRR2 := httptest.NewRecorder()
	list(listRR2, listReq2)
	var listed2 types.CommentsResp
	if err := json.Unmarshal(listRR2.Body.Bytes(), &listed2); err != nil {
		t.Fatalf("unmarshal CommentsResp: %v", err)
	}
	if listed2.Total != 3 || len(listed2.Comments) != 3 {
		t.Fatalf("list total = %d comments = %d, want 3", listed2.Total, len(listed2.Comments))
	}

	del := svcCtx.Auth(comment.DeleteCommentHandler(svcCtx))

	// 读者删作者评论 → 403
	strangerReq := httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+postId+"/comments/"+parentId, nil)
	strangerReq = pathvar.WithVars(strangerReq, map[string]string{"id": postId, "cid": parentId})
	strangerReq.Header.Set("Authorization", "Bearer "+readerToken)
	strangerRR := httptest.NewRecorder()
	del(strangerRR, strangerReq)
	if strangerRR.Code != http.StatusForbidden {
		t.Fatalf("stranger delete: expected 403, got %d body=%s", strangerRR.Code, strangerRR.Body.String())
	}

	// 作者删自己的评论 → 200，列表回落 2 条
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+postId+"/comments/"+parentId, nil)
	delReq = pathvar.WithVars(delReq, map[string]string{"id": postId, "cid": parentId})
	delReq.Header.Set("Authorization", "Bearer "+authorToken)
	delRR := httptest.NewRecorder()
	del(delRR, delReq)
	if delRR.Code != http.StatusOK {
		t.Fatalf("author delete: expected 200, got %d body=%s", delRR.Code, delRR.Body.String())
	}
	listReq3 := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+postId+"/comments", nil)
	listReq3 = pathvar.WithVars(listReq3, map[string]string{"id": postId})
	listRR3 := httptest.NewRecorder()
	list(listRR3, listReq3)
	var listed3 types.CommentsResp
	if err := json.Unmarshal(listRR3.Body.Bytes(), &listed3); err != nil {
		t.Fatalf("unmarshal CommentsResp: %v", err)
	}
	if listed3.Total != 2 || len(listed3.Comments) != 2 {
		t.Fatalf("list after delete = %d/%d, want 2", listed3.Total, len(listed3.Comments))
	}

	// 三端点 Parse 失败分支 → 400 信封
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+postId+"/comments", bytes.NewBufferString(`{"content":}`))
	badReq.Header.Set("Content-Type", "application/json")
	badReq = pathvar.WithVars(badReq, map[string]string{"id": postId})
	badReq.Header.Set("Authorization", "Bearer "+readerToken)
	badRR := httptest.NewRecorder()
	postComment(badRR, badReq)
	if badRR.Code != http.StatusBadRequest {
		t.Fatalf("bad body: expected 400, got %d body=%s", badRR.Code, badRR.Body.String())
	}

	badListReq := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+postId+"/comments?limit=abc", nil)
	badListReq = pathvar.WithVars(badListReq, map[string]string{"id": postId})
	badListRR := httptest.NewRecorder()
	list(badListRR, badListReq)
	if badListRR.Code != http.StatusBadRequest {
		t.Fatalf("bad query: expected 400, got %d body=%s", badListRR.Code, badListRR.Body.String())
	}

	badDelReq := httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+postId+"/comments/abc", nil)
	badDelReq = pathvar.WithVars(badDelReq, map[string]string{"id": postId, "cid": "abc"})
	badDelReq.Header.Set("Authorization", "Bearer "+readerToken)
	badDelRR := httptest.NewRecorder()
	del(badDelRR, badDelReq)
	if badDelRR.Code != http.StatusBadRequest {
		t.Fatalf("bad cid: expected 400, got %d body=%s", badDelRR.Code, badDelRR.Body.String())
	}

	// 不存在的帖子列表评论 → 404
	ghostListReq := httptest.NewRequest(http.MethodGet, "/api/v1/posts/9999/comments", nil)
	ghostListReq = pathvar.WithVars(ghostListReq, map[string]string{"id": "9999"})
	ghostListRR := httptest.NewRecorder()
	list(ghostListRR, ghostListReq)
	if ghostListRR.Code != http.StatusNotFound {
		t.Fatalf("ghost list: expected 404, got %d body=%s", ghostListRR.Code, ghostListRR.Body.String())
	}
}

// brokenNotificationRepo 通知仓储故障桩：三个通知端点的 logic 错误分支
// 需要仓储报错才可达，此处整体替换 NotificationRepo 注入。
type brokenNotificationRepo struct{}

func (brokenNotificationRepo) CreateNotificationsTable() error { return errors.New("boom") }
func (brokenNotificationRepo) Create(n *types.Notification) error {
	return errors.New("boom")
}
func (brokenNotificationRepo) ListByUser(userId, limit, offset int64) ([]*types.Notification, int64, error) {
	return nil, 0, errors.New("boom")
}
func (brokenNotificationRepo) CountUnread(userId int64) (int64, error) {
	return 0, errors.New("boom")
}
func (brokenNotificationRepo) MarkAllRead(userId int64) error  { return errors.New("boom") }
func (brokenNotificationRepo) MarkRead(userId, id int64) error { return errors.New("boom") }

// TestNotificationEndpoints_RepoError 仓储故障 → logic 错误分支 → httperr
// 兜底信封（非哨兵错误一律 400）。
func TestNotificationEndpoints_RepoError(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, secret := newTestServiceContext(t)
	token := signToken(t, secret, 7, "作者甲")
	svcCtx.NotificationRepo = brokenNotificationRepo{}

	list := svcCtx.Auth(notification.ListNotificationsHandler(svcCtx))
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRR := httptest.NewRecorder()
	list(listRR, listReq)
	if listRR.Code != http.StatusBadRequest {
		t.Fatalf("list on broken repo: expected 400, got %d body=%s", listRR.Code, listRR.Body.String())
	}

	unread := svcCtx.Auth(notification.GetUnreadCountHandler(svcCtx))
	unreadReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
	unreadReq.Header.Set("Authorization", "Bearer "+token)
	unreadRR := httptest.NewRecorder()
	unread(unreadRR, unreadReq)
	if unreadRR.Code != http.StatusBadRequest {
		t.Fatalf("unread-count on broken repo: expected 400, got %d body=%s", unreadRR.Code, unreadRR.Body.String())
	}

	markRead := svcCtx.Auth(notification.MarkAllReadHandler(svcCtx))
	markReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read-all", nil)
	markReq.Header.Set("Authorization", "Bearer "+token)
	markRR := httptest.NewRecorder()
	markRead(markRR, markReq)
	if markRR.Code != http.StatusBadRequest {
		t.Fatalf("read-all on broken repo: expected 400, got %d body=%s", markRR.Code, markRR.Body.String())
	}
}

// TestMessageEndpoints_Flow 私信五端点走真 handler：A 发 → B 会话未读 →
// 线程明细 → B 置已读归零 → 越权线程隔离。
func TestMessageEndpoints_Flow(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, secret := newTestServiceContext(t)
	aToken := signToken(t, secret, 7, "作者甲")
	bToken := signToken(t, secret, 8, "读者乙")

	// A → B 发私信
	send := svcCtx.Auth(message.SendDirectMessageHandler(svcCtx))
	sendReq := httptest.NewRequest(http.MethodPost, "/api/v1/messages", bytes.NewBufferString(`{"recipient_id":8,"content":"第一条私信"}`))
	sendReq.Header.Set("Content-Type", "application/json")
	sendReq.Header.Set("Authorization", "Bearer "+aToken)
	sendRR := httptest.NewRecorder()
	send(sendRR, sendReq)
	if sendRR.Code != http.StatusOK {
		t.Fatalf("send: expected 200, got %d body=%s", sendRR.Code, sendRR.Body.String())
	}

	// 无 token 发送 → 401
	bareRR := httptest.NewRecorder()
	send(bareRR, httptest.NewRequest(http.MethodPost, "/api/v1/messages", bytes.NewBufferString(`{"recipient_id":8,"content":"x"}`)))
	if bareRR.Code != http.StatusUnauthorized {
		t.Fatalf("send without token: expected 401, got %d", bareRR.Code)
	}

	// B 未读总数 → 1
	unread := svcCtx.Auth(message.GetDMUnreadCountHandler(svcCtx))
	unreadReq := httptest.NewRequest(http.MethodGet, "/api/v1/messages/unread-count", nil)
	unreadReq.Header.Set("Authorization", "Bearer "+bToken)
	unreadRR2 := httptest.NewRecorder()
	unread(unreadRR2, unreadReq)
	if unreadRR2.Code != http.StatusOK {
		t.Fatalf("unread-count: expected 200, got %d", unreadRR2.Code)
	}
	var unreadResp types.UnreadCountResp
	if err := json.Unmarshal(unreadRR2.Body.Bytes(), &unreadResp); err != nil || unreadResp.Count != 1 {
		t.Fatalf("unread = %s (%v)", unreadRR2.Body.String(), err)
	}

	// B 会话列表 → peer 7 未读 1
	convs := svcCtx.Auth(message.ListDMConversationsHandler(svcCtx))
	convsReq := httptest.NewRequest(http.MethodGet, "/api/v1/messages/conversations", nil)
	convsReq.Header.Set("Authorization", "Bearer "+bToken)
	convsRR := httptest.NewRecorder()
	convs(convsRR, convsReq)
	var convsResp types.DMConversationsResp
	if err := json.Unmarshal(convsRR.Body.Bytes(), &convsResp); err != nil {
		t.Fatalf("unmarshal conversations: %v", err)
	}
	if len(convsResp.Conversations) != 1 || convsResp.Conversations[0].PeerId != 7 || convsResp.Conversations[0].Unread != 1 {
		t.Fatalf("conversations = %#v", convsResp)
	}

	// B 拉线程 → 1 条
	thread := svcCtx.Auth(message.GetDMThreadHandler(svcCtx))
	threadReq := httptest.NewRequest(http.MethodGet, "/api/v1/messages/thread/7", nil)
	threadReq = pathvar.WithVars(threadReq, map[string]string{"peer_id": "7"})
	threadReq.Header.Set("Authorization", "Bearer "+bToken)
	threadRR := httptest.NewRecorder()
	thread(threadRR, threadReq)
	var threadResp types.DMThreadResp
	if err := json.Unmarshal(threadRR.Body.Bytes(), &threadResp); err != nil || threadResp.Total != 1 ||
		len(threadResp.Messages) != 1 || threadResp.Messages[0].Content != "第一条私信" {
		t.Fatalf("thread = %s (%v)", threadRR.Body.String(), err)
	}

	// B 置已读 → 剩余未读 0
	mark := svcCtx.Auth(message.MarkDMThreadReadHandler(svcCtx))
	markReq := httptest.NewRequest(http.MethodPost, "/api/v1/messages/thread/7/read", nil)
	markReq = pathvar.WithVars(markReq, map[string]string{"peer_id": "7"})
	markReq.Header.Set("Authorization", "Bearer "+bToken)
	markRR := httptest.NewRecorder()
	mark(markRR, markReq)
	var markResp types.UnreadCountResp
	if err := json.Unmarshal(markRR.Body.Bytes(), &markResp); err != nil || markResp.Count != 0 {
		t.Fatalf("mark read = %s (%v)", markRR.Body.String(), err)
	}

	// 越权：B 查与 9 的线程 → 空而非 A/B 往来
	otherReq := httptest.NewRequest(http.MethodGet, "/api/v1/messages/thread/9", nil)
	otherReq = pathvar.WithVars(otherReq, map[string]string{"peer_id": "9"})
	otherReq.Header.Set("Authorization", "Bearer "+bToken)
	otherRR := httptest.NewRecorder()
	thread(otherRR, otherReq)
	var otherResp types.DMThreadResp
	if err := json.Unmarshal(otherRR.Body.Bytes(), &otherResp); err != nil || otherResp.Total != 0 {
		t.Fatalf("foreign thread = %s (%v)", otherRR.Body.String(), err)
	}
}
