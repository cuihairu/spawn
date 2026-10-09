package follow

import (
	"testing"

	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
)

// listFollowNotificationsOf 读某收件人的全部通知（触发点测试断言用）。
func listFollowNotificationsOf(t *testing.T, s *svc.ServiceContext, userId int64) []*types.Notification {
	t.Helper()
	list, _, err := s.NotificationRepo.ListByUser(userId, 50, 0)
	if err != nil {
		t.Fatalf("list notifications for %d: %v", userId, err)
	}
	return list
}

func TestFollowUserNotifiesFollowee(t *testing.T) {
	s := newFollowSvcCtx(t)

	if _, err := NewFollowUserLogic(authCtx(8), s).FollowUser(&types.FollowUserReq{UserId: 7}); err != nil {
		t.Fatalf("follow: %v", err)
	}

	list := listFollowNotificationsOf(t, s, 7)
	if len(list) != 1 {
		t.Fatalf("followee notifications=%d, want 1", len(list))
	}
	if list[0].Type != "follow_user" || list[0].ActorId != 8 || list[0].TargetId != 7 {
		t.Fatalf("notification mismatch: %+v", list[0])
	}
}

func TestFollowUserRepeatedDoesNotDuplicateNotification(t *testing.T) {
	s := newFollowSvcCtx(t)

	// 重复关注幂等返回 false，不再产生通知
	for i := 0; i < 2; i++ {
		if _, err := NewFollowUserLogic(authCtx(8), s).FollowUser(&types.FollowUserReq{UserId: 7}); err != nil {
			t.Fatalf("follow #%d: %v", i+1, err)
		}
	}
	if got := listFollowNotificationsOf(t, s, 7); len(got) != 1 {
		t.Fatalf("followee notifications=%d, want 1", len(got))
	}
}

func TestFollowUserFollowerNotNotified(t *testing.T) {
	s := newFollowSvcCtx(t)

	if _, err := NewFollowUserLogic(authCtx(8), s).FollowUser(&types.FollowUserReq{UserId: 7}); err != nil {
		t.Fatalf("follow: %v", err)
	}
	if got := listFollowNotificationsOf(t, s, 8); len(got) != 0 {
		t.Fatalf("follower should not be notified: %+v", got)
	}
}
