package post

import (
	"errors"
	"fmt"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

// recordingPublisher 记录 Publish 调用（逻辑层同步调用，无并发）。
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

// TestLikePostPublishesRealtimeEvent 点赞通知落库成功后推 WS 事件给作者；
// 自赞无通知也无事件。
func TestLikePostPublishesRealtimeEvent(t *testing.T) {
	s := newTestServiceContext(t)
	rec := &recordingPublisher{}
	s.Realtime = rec

	p, err := s.PostRepo.Create(1, 7, "作者甲", &types.CreatePostReq{Title: "被赞", Content: "正文"})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	if _, err := NewLikePostLogic(authContext(8, "读者乙"), s).LikePost(&types.LikePostReq{Id: p.Id}); err != nil {
		t.Fatalf("like: %v", err)
	}
	if len(rec.events) != 1 || rec.events[0] != "[7] notification like_post" {
		t.Fatalf("events = %v, want one like_post push to [7]", rec.events)
	}

	// 自赞：无通知分支，也无事件
	if _, err := NewLikePostLogic(authContext(7, "作者甲"), s).LikePost(&types.LikePostReq{Id: p.Id}); err != nil {
		t.Fatalf("self like: %v", err)
	}
	if len(rec.events) != 1 {
		t.Fatalf("self like must not publish, events = %v", rec.events)
	}
}

// TestLikePostNotificationFailureSkipsRealtime 通知落库失败则不推事件
// （WS 帧与未读列表保持一致，防幽灵红点）。
func TestLikePostNotificationFailureSkipsRealtime(t *testing.T) {
	s := newTestServiceContext(t)
	rec := &recordingPublisher{}
	s.Realtime = rec
	s.NotificationRepo = failingNotificationStoreForPost{err: errors.New("db down")} // 落库必败

	p, err := s.PostRepo.Create(1, 7, "作者甲", &types.CreatePostReq{Title: "被赞", Content: "正文"})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	if _, err := NewLikePostLogic(authContext(8, "读者乙"), s).LikePost(&types.LikePostReq{Id: p.Id}); err != nil {
		t.Fatalf("like must still succeed: %v", err)
	}
	if len(rec.events) != 0 {
		t.Fatalf("events = %v, want none on notification failure", rec.events)
	}
}
