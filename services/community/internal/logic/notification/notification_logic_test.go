package notification

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/community/internal/config"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
)

// newTestServiceContext 与 comment/post 包同款：真实服务上下文 + SQLite 临时库。
func newTestServiceContext(t *testing.T) *svc.ServiceContext {
	t.Helper()

	var c config.Config
	c.Auth.JWTSecret = "test-jwt-secret"
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "community.db")

	return svc.NewServiceContext(c)
}

func authContext(userId int64, username string) context.Context {
	ctx := context.WithValue(context.Background(), "user_id", userId)
	return context.WithValue(ctx, "username", username)
}

func seedNotification(t *testing.T, s *svc.ServiceContext, recipient, actor int64, typ string, target int64) {
	t.Helper()
	if err := s.NotificationRepo.Create(&types.Notification{
		UserId: recipient, ActorId: actor, ActorName: "昵称", Type: typ,
		TargetId: target, Content: "测试通知", CreatedAt: "2026-10-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed notification: %v", err)
	}
}

func TestListNotifications(t *testing.T) {
	s := newTestServiceContext(t)
	seedNotification(t, s, 7, 8, "like_post", 101)
	seedNotification(t, s, 7, 9, "comment_post", 101)
	seedNotification(t, s, 8, 7, "like_post", 102)

	resp, err := NewListNotificationsLogic(authContext(7, "作者甲"), s).ListNotifications(&types.ListNotificationsReq{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.Total != 2 || len(resp.Notifications) != 2 {
		t.Fatalf("total=%d len=%d, want 2", resp.Total, len(resp.Notifications))
	}
	// 收件人隔离：8 的通知不出现在 7 的列表
	if resp.Notifications[0].TargetId != 101 || resp.Notifications[1].ActorId != 8 {
		t.Fatalf("order/actor mismatch: %+v", resp.Notifications)
	}
}

func TestListNotifications_EmptyNotNil(t *testing.T) {
	s := newTestServiceContext(t)

	resp, err := NewListNotificationsLogic(authContext(7, "作者甲"), s).ListNotifications(&types.ListNotificationsReq{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.Notifications == nil || len(resp.Notifications) != 0 || resp.Total != 0 {
		t.Fatalf("want empty non-nil slice, got %#v", resp)
	}
}

func TestListNotifications_PagingClamp(t *testing.T) {
	s := newTestServiceContext(t)
	for i := 0; i < 3; i++ {
		seedNotification(t, s, 7, int64(100+i), "like_post", int64(200+i))
	}

	// limit<=0 → 20；offset<0 → 0
	resp, err := NewListNotificationsLogic(authContext(7, "作者甲"), s).ListNotifications(&types.ListNotificationsReq{Limit: -1, Offset: -5})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.Total != 3 || len(resp.Notifications) != 3 {
		t.Fatalf("total=%d len=%d, want 3", resp.Total, len(resp.Notifications))
	}

	// limit>100 → 100（此处仅 3 条，全量返回）
	resp, err = NewListNotificationsLogic(authContext(7, "作者甲"), s).ListNotifications(&types.ListNotificationsReq{Limit: 500})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Notifications) != 3 {
		t.Fatalf("len=%d, want 3", len(resp.Notifications))
	}

	// 第二页 offset=2 只剩 1 条
	resp, err = NewListNotificationsLogic(authContext(7, "作者甲"), s).ListNotifications(&types.ListNotificationsReq{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Notifications) != 1 || resp.Notifications[0].TargetId != 200 {
		t.Fatalf("page2 mismatch: %+v", resp.Notifications)
	}
}

func TestListNotifications_Unauthorized(t *testing.T) {
	s := newTestServiceContext(t)

	_, err := NewListNotificationsLogic(context.Background(), s).ListNotifications(&types.ListNotificationsReq{})
	if err == nil {
		t.Fatalf("want 401 for missing auth, got nil")
	}
}

func TestGetUnreadCount_AndMarkAllRead(t *testing.T) {
	s := newTestServiceContext(t)
	seedNotification(t, s, 7, 8, "like_post", 101)
	seedNotification(t, s, 7, 9, "follow_user", 7)

	resp, err := NewGetUnreadCountLogic(authContext(7, "作者甲"), s).GetUnreadCount()
	if err != nil || resp.Count != 2 {
		t.Fatalf("count=%+v err=%v, want 2", resp, err)
	}

	if _, err := NewMarkAllReadLogic(authContext(7, "作者甲"), s).MarkAllRead(); err != nil {
		t.Fatalf("mark all read: %v", err)
	}
	resp, err = NewGetUnreadCountLogic(authContext(7, "作者甲"), s).GetUnreadCount()
	if err != nil || resp.Count != 0 {
		t.Fatalf("count after mark=%+v err=%v, want 0", resp, err)
	}
}

func TestGetUnreadCount_Unauthorized(t *testing.T) {
	s := newTestServiceContext(t)

	if _, err := NewGetUnreadCountLogic(context.Background(), s).GetUnreadCount(); err == nil {
		t.Fatalf("want 401 for missing auth, got nil")
	}
}

func TestMarkAllRead_Unauthorized(t *testing.T) {
	s := newTestServiceContext(t)

	if _, err := NewMarkAllReadLogic(context.Background(), s).MarkAllRead(); err == nil {
		t.Fatalf("want 401 for missing auth, got nil")
	}
}
