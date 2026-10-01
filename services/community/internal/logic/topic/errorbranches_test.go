package topic

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/community/internal/config"
	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
)

// newTopicSvcCtx 构造逻辑层单测服务上下文：SQLite 文件库落在 t.TempDir()。
func newTopicSvcCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()

	var c config.Config
	c.Auth.JWTSecret = "topic-branch-secret"
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "community.db")

	return svc.NewServiceContext(c)
}

// failingTopicStore 仅 Create 投毒的故障注入仓储（其余方法沿真实链路不可达，
// 嵌入接口即可满足 TopicStore）。
type failingTopicStore struct {
	model.TopicStore
	createErr error
}

func (s failingTopicStore) Create(*types.CreateTopicReq) (*types.Topic, error) {
	return nil, s.createErr
}

// authCtx 模拟鉴权中间件注入的 user_id（键名与 middleware 保持一致）。
func authCtx(userId int64) context.Context {
	return context.WithValue(context.Background(), "user_id", userId)
}

// requireStatus 断言逻辑层返回的 httperr 状态码。
func requireStatus(t *testing.T, err error, want int) {
	t.Helper()
	var appErr *httperr.Error
	if !errors.As(err, &appErr) || appErr.Status != want {
		t.Fatalf("err = %v (%T), want status %d", err, err, want)
	}
}

// --- GetTopics：分页参数钳制 ---

func TestGetTopics_NormalizesPaging(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)

	resp, err := NewGetTopicsLogic(context.Background(), svcCtx).
		GetTopics(&types.GetTopicsReq{Limit: 0, Offset: -5}) // limit<=0 → 20；offset<0 → 0
	if err != nil || resp == nil || len(resp.Topics) == 0 {
		t.Fatalf("resp=%+v err=%v, want default-paged seeds", resp, err)
	}

	resp2, err := NewGetTopicsLogic(context.Background(), svcCtx).
		GetTopics(&types.GetTopicsReq{Limit: 1000}) // limit>100 → 100
	if err != nil || resp2 == nil {
		t.Fatalf("resp=%+v err=%v, want capped page", resp2, err)
	}
}

// --- GetTopic：非法请求 ---

func TestGetTopic_InvalidRequest(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	_, err := NewGetTopicLogic(context.Background(), svcCtx).GetTopic(nil)
	requireStatus(t, err, 400)
}

// --- GetFollowingTopics：未认证 / 关注的话题已被删除 ---

func TestGetFollowingTopics_Unauthorized(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	_, err := NewGetFollowingTopicsLogic(context.Background(), svcCtx).GetFollowingTopics()
	requireStatus(t, err, 401)
}

func TestGetFollowingTopics_SkipsRemovedTopic(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	svcCtx.FollowRepo.FollowTopic(7, 99999) // 直接登记一个不存在话题的关注

	resp, err := NewGetFollowingTopicsLogic(authCtx(7), svcCtx).GetFollowingTopics()
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(resp.Topics) != 0 {
		t.Fatalf("topics = %+v, want removed topic skipped", resp.Topics)
	}
}

// --- FollowTopic / UnfollowTopic：未认证与非法参数 ---

func TestFollowTopic_Unauthorized(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	_, err := NewFollowTopicLogic(context.Background(), svcCtx).FollowTopic(&types.FollowTopicReq{TopicId: 1})
	requireStatus(t, err, 401)
}

func TestFollowTopic_MissingTopicId(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	_, err := NewFollowTopicLogic(authCtx(1), svcCtx).FollowTopic(&types.FollowTopicReq{TopicId: 0})
	requireStatus(t, err, 400)
}

func TestUnfollowTopic_Unauthorized(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	_, err := NewUnfollowTopicLogic(context.Background(), svcCtx).UnfollowTopic(&types.FollowTopicReq{TopicId: 1})
	requireStatus(t, err, 401)
}

func TestUnfollowTopic_MissingTopicId(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	_, err := NewUnfollowTopicLogic(authCtx(1), svcCtx).UnfollowTopic(&types.FollowTopicReq{TopicId: 0})
	requireStatus(t, err, 400)
}

// --- CreateTopic：未认证 / 空请求 / 空名称 / 落盘失败 ---

func TestCreateTopic_Unauthorized(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	_, err := NewCreateTopicLogic(context.Background(), svcCtx).
		CreateTopic(&types.CreateTopicReq{Name: "x"})
	requireStatus(t, err, 401)
}

func TestCreateTopic_NilRequest(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	_, err := NewCreateTopicLogic(authCtx(1), svcCtx).CreateTopic(nil)
	requireStatus(t, err, 400)
}

func TestCreateTopic_BlankName(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	_, err := NewCreateTopicLogic(authCtx(1), svcCtx).CreateTopic(&types.CreateTopicReq{Name: "   "})
	requireStatus(t, err, 400)
}

func TestCreateTopic_SaveError(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	svcCtx.TopicRepo = failingTopicStore{createErr: errors.New("disk on fire")}

	_, err := NewCreateTopicLogic(authCtx(1), svcCtx).CreateTopic(&types.CreateTopicReq{Name: "x"})
	if err == nil {
		t.Fatal("expected CreateTopic to fail when topic persistence fails")
	}
}
