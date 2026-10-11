package follow

import (
	"fmt"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

type recordingPublisher struct {
	events []string
}

func (r *recordingPublisher) Publish(eventType string, userIds []int64, payload any) {
	nt := "<nil>"
	if n, ok := payload.(*types.Notification); ok {
		nt = n.Type
	}
	r.events = append(r.events, fmt.Sprintf("%v %s %s", userIds, eventType, nt))
}

// TestFollowUserPublishesRealtimeEvent 首次关注推一帧给被关注者；
// 重复关注幂等不再推。
func TestFollowUserPublishesRealtimeEvent(t *testing.T) {
	s := newFollowSvcCtx(t)
	rec := &recordingPublisher{}
	s.Realtime = rec

	if _, err := NewFollowUserLogic(authCtx(8), s).FollowUser(&types.FollowUserReq{UserId: 7}); err != nil {
		t.Fatalf("follow: %v", err)
	}
	if len(rec.events) != 1 || rec.events[0] != "[7] notification follow_user" {
		t.Fatalf("events = %v, want one follow_user push to [7]", rec.events)
	}

	if _, err := NewFollowUserLogic(authCtx(8), s).FollowUser(&types.FollowUserReq{UserId: 7}); err != nil {
		t.Fatalf("re-follow: %v", err)
	}
	if len(rec.events) != 1 {
		t.Fatalf("re-follow must not publish, events = %v", rec.events)
	}
}
