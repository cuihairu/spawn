package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tappi/tappi/services/community/internal/config"
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
}
