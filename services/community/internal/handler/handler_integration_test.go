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
	dir := t.TempDir()

	var c config.Config
	c.Auth.JWTSecret = secret
	c.DataSource.TopicsFile = filepath.Join(dir, "topics.json")
	c.DataSource.PostsFile = filepath.Join(dir, "posts.json")
	c.DataSource.FollowsFile = filepath.Join(dir, "follows.json")

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
