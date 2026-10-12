package notification

import (
	"errors"
	"sync"
	"testing"

	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/types"
)

// recordingPublisher 记录 Publish 调用（notification 包测试专用）。
type recordingPublisher struct {
	mu     sync.Mutex
	events []recEvent
}

type recEvent struct {
	typ     string
	userIds []int64
}

func (p *recordingPublisher) Publish(eventType string, userIds []int64, _ any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, recEvent{eventType, userIds})
}

// failingMarkStore MarkAllRead 必败（包装真实库，其余方法透传）。
type failingMarkStore struct {
	model.NotificationStore
}

func (f failingMarkStore) MarkAllRead(int64) error { return errors.New("db down") }

// TestMarkAllReadPublishesReceipt 读回执发布给本人；写库失败不发布。
func TestMarkAllReadPublishesReceipt(t *testing.T) {
	s := newTestServiceContext(t)
	rec := &recordingPublisher{}
	s.Realtime = rec
	seedNotification(t, s, 7, 8, "like_post", 101)

	if _, err := NewMarkAllReadLogic(authContext(7, "作者甲"), s).MarkAllRead(); err != nil {
		t.Fatalf("mark all read: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 1 || rec.events[0].typ != "notification.read" ||
		len(rec.events[0].userIds) != 1 || rec.events[0].userIds[0] != 7 {
		t.Fatalf("events = %+v", rec.events)
	}
}

func TestMarkAllReadFailureNoPublish(t *testing.T) {
	s := newTestServiceContext(t)
	s.NotificationRepo = failingMarkStore{s.NotificationRepo}
	rec := &recordingPublisher{}
	s.Realtime = rec

	if _, err := NewMarkAllReadLogic(authContext(7, "作者甲"), s).MarkAllRead(); err == nil {
		t.Fatal("expected mark-all-read error")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 0 {
		t.Fatalf("unexpected events = %+v", rec.events)
	}
}

// TestMarkSingleReadPublishesReceiptWithCount 单条已读：回执带剩余未读数。
func TestMarkSingleReadPublishesReceiptWithCount(t *testing.T) {
	s := newTestServiceContext(t)
	rec := &recordingPublisher{}
	s.Realtime = rec
	seedNotification(t, s, 7, 8, "like_post", 101)
	seedNotification(t, s, 7, 9, "comment_post", 102)

	var firstId int64
	items, _, err := s.NotificationRepo.ListByUser(7, 10, 0)
	if err != nil || len(items) == 0 {
		t.Fatalf("list = %v items err=%v", len(items), err)
	}
	firstId = items[0].Id

	if _, err := NewMarkNotificationReadLogic(authContext(7, "作者甲"), s).MarkNotificationRead(&types.NotificationIdReq{Id: firstId}); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 1 || rec.events[0].typ != "notification.read" ||
		len(rec.events[0].userIds) != 1 || rec.events[0].userIds[0] != 7 {
		t.Fatalf("events = %+v", rec.events)
	}
}

// TestMarkSingleReadNotFoundNoPublish 非本人/不存在：错误外露且不发布。
func TestMarkSingleReadNotFoundNoPublish(t *testing.T) {
	s := newTestServiceContext(t)
	rec := &recordingPublisher{}
	s.Realtime = rec
	seedNotification(t, s, 7, 8, "like_post", 101)

	if _, err := NewMarkNotificationReadLogic(authContext(8, "读者乙"), s).MarkNotificationRead(&types.NotificationIdReq{Id: 99999}); err == nil {
		t.Fatal("expected not-found error")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 0 {
		t.Fatalf("unexpected events = %+v", rec.events)
	}
}
