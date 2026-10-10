package follow

import (
	"context"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

func TestGetFollowingUsers_Unauthorized(t *testing.T) {
	_, err := NewGetFollowingUsersLogic(context.Background(), newFollowSvcCtx(t)).
		GetFollowingUsers()
	requireStatus(t, err, 401)
}

func TestGetFollowingUsers_Empty(t *testing.T) {
	resp, err := NewGetFollowingUsersLogic(authCtx(1), newFollowSvcCtx(t)).
		GetFollowingUsers()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 空关注返回空数组而非 null（客户端契约）
	if resp == nil || resp.UserIds == nil {
		t.Fatalf("UserIds = %+v, want non-nil empty slice", resp)
	}
	if len(resp.UserIds) != 0 {
		t.Fatalf("UserIds = %v, want empty", resp.UserIds)
	}
}

func TestGetFollowingUsers_ListsFollowedIdsAscending(t *testing.T) {
	svcCtx := newFollowSvcCtx(t)
	// 乱序关注 3 与 1，列表须升序（与 ListFollowingTopicIds 同口径）
	for _, target := range []int64{3, 1} {
		if _, err := NewFollowUserLogic(authCtx(7), svcCtx).
			FollowUser(&types.FollowUserReq{UserId: target}); err != nil {
			t.Fatalf("follow %d: %v", target, err)
		}
	}
	resp, err := NewGetFollowingUsersLogic(authCtx(7), svcCtx).
		GetFollowingUsers()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []int64{1, 3}
	if len(resp.UserIds) != len(want) {
		t.Fatalf("UserIds = %v, want %v", resp.UserIds, want)
	}
	for i := range want {
		if resp.UserIds[i] != want[i] {
			t.Fatalf("UserIds = %v, want %v", resp.UserIds, want)
		}
	}
}

func TestFollowUser_SelfFollow(t *testing.T) {
	_, err := NewFollowUserLogic(authCtx(5), newFollowSvcCtx(t)).
		FollowUser(&types.FollowUserReq{UserId: 5})
	requireStatus(t, err, 400)
}
