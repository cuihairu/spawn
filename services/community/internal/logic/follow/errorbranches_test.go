package follow

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/community/internal/config"
	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
)

// newFollowSvcCtx 构造逻辑层单测服务上下文：SQLite 文件库落在 t.TempDir()。
func newFollowSvcCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()

	var c config.Config
	c.Auth.JWTSecret = "follow-branch-secret"
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "community.db")

	return svc.NewServiceContext(c)
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

func TestFollowUser_Unauthorized(t *testing.T) {
	_, err := NewFollowUserLogic(context.Background(), newFollowSvcCtx(t)).
		FollowUser(&types.FollowUserReq{UserId: 2})
	requireStatus(t, err, 401)
}

func TestFollowUser_MissingUserId(t *testing.T) {
	_, err := NewFollowUserLogic(authCtx(1), newFollowSvcCtx(t)).
		FollowUser(&types.FollowUserReq{UserId: 0})
	requireStatus(t, err, 400)
}

func TestUnfollowUser_Unauthorized(t *testing.T) {
	_, err := NewUnfollowUserLogic(context.Background(), newFollowSvcCtx(t)).
		UnfollowUser(&types.FollowUserReq{UserId: 2})
	requireStatus(t, err, 401)
}

func TestUnfollowUser_MissingUserId(t *testing.T) {
	_, err := NewUnfollowUserLogic(authCtx(1), newFollowSvcCtx(t)).
		UnfollowUser(&types.FollowUserReq{UserId: 0})
	requireStatus(t, err, 400)
}

func TestUnfollowUser_SelfUnfollow(t *testing.T) {
	_, err := NewUnfollowUserLogic(authCtx(5), newFollowSvcCtx(t)).
		UnfollowUser(&types.FollowUserReq{UserId: 5})
	requireStatus(t, err, 400)
}
