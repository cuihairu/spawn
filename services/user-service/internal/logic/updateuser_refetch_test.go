package logic

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/tappi/tappi/services/user-service/internal/types"
	"github.com/tappi/tappi/services/user-service/model"
)

// refetchFaultStore 内嵌真实仓储，仅覆写 FindOne 注入「更新后回读」故障，
// 触达 UpdateUserInfoLogic 更新成功后二次 FindOne 的两个分支。
type refetchFaultStore struct {
	model.UserStore
	calls int
	mode  string // "findErr"：二次回读报错；"nullNickname"：二次回读昵称为 NULL
}

func (f *refetchFaultStore) FindOne(id int64) (*model.User, error) {
	f.calls++
	if f.mode == "findErr" && f.calls == 2 {
		return nil, errors.New("transient db failure after update")
	}
	u, err := f.UserStore.FindOne(id)
	if err != nil {
		return nil, err
	}
	if f.mode == "nullNickname" && f.calls == 2 {
		c := *u
		c.Nickname = sql.NullString{}
		return &c, nil
	}
	return u, nil
}

// TestUpdateUserInfoLogic_RefetchFails500 更新成功后二次回读失败
// （生产瞬时故障；对称 DB 故障够不到此分支）→ 500「更新成功，但获取信息失败」。
func TestUpdateUserInfoLogic_RefetchFails500(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	id := insertTestUser(t, svcCtx.DB, "alice", "alice@example.com", "hashed", "Alice")
	svcCtx.UserModel = &refetchFaultStore{UserStore: svcCtx.UserModel, mode: "findErr"}

	resp, err := NewUpdateUserInfoLogic(context.Background(), svcCtx).UpdateUserInfo(
		&types.UpdateUserInfoRequest{Id: id, Email: "alice@new.com", Nickname: "Alicia"})
	if err != nil {
		t.Fatalf("expected envelope, got error %v", err)
	}
	if resp.Code != 500 || resp.Message != "更新成功，但获取信息失败" {
		t.Fatalf("code=%d message=%q, want 500 更新成功，但获取信息失败", resp.Code, resp.Message)
	}
}

// TestUpdateUserInfoLogic_NullNicknameFallback 二次回读昵称为 NULL
// （历史行无昵称）→ 走 nickname 回退分支，昵称取 username。
func TestUpdateUserInfoLogic_NullNicknameFallback(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	id := insertTestUser(t, svcCtx.DB, "alice", "alice@example.com", "hashed", "Alice")
	svcCtx.UserModel = &refetchFaultStore{UserStore: svcCtx.UserModel, mode: "nullNickname"}

	resp, err := NewUpdateUserInfoLogic(context.Background(), svcCtx).UpdateUserInfo(
		&types.UpdateUserInfoRequest{Id: id, Email: "alice@new.com", Nickname: "Alicia"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Code != 200 {
		t.Fatalf("code=%d message=%q, want 200", resp.Code, resp.Message)
	}
	if info := resp.Data.(*types.UserInfoResponse); info.Nickname != "alice" {
		t.Fatalf("nickname = %q, want username fallback", info.Nickname)
	}
}
