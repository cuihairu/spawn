package notification

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/types"
)

// failingNotificationStore 仅单方法投毒的故障通知仓储，其余方法不可达
// 或无需覆盖，嵌入真实实现以保持接口满足。
type failingNotificationStore struct {
	model.NotificationStore
	listErr, countErr, markReadErr error
}

func (s failingNotificationStore) ListByUser(int64, int64, int64) ([]*types.Notification, int64, error) {
	return nil, 0, s.listErr
}

func (s failingNotificationStore) CountUnread(int64) (int64, error) {
	return 0, s.countErr
}

func (s failingNotificationStore) MarkAllRead(int64) error {
	return s.markReadErr
}

func TestListNotifications_RepoErrorPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	s.NotificationRepo = failingNotificationStore{NotificationStore: s.NotificationRepo, listErr: errors.New("disk on fire")}

	_, err := NewListNotificationsLogic(authContext(7, "作者甲"), s).
		ListNotifications(&types.ListNotificationsReq{})
	if err == nil {
		t.Fatal("list notifications repo error must propagate")
	}
}

func TestGetUnreadCount_RepoErrorPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	s.NotificationRepo = failingNotificationStore{NotificationStore: s.NotificationRepo, countErr: errors.New("disk on fire")}

	_, err := NewGetUnreadCountLogic(authContext(7, "作者甲"), s).GetUnreadCount()
	if err == nil {
		t.Fatal("unread count repo error must propagate")
	}
}

func TestMarkAllRead_RepoErrorPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	s.NotificationRepo = failingNotificationStore{NotificationStore: s.NotificationRepo, markReadErr: errors.New("disk on fire")}

	_, err := NewMarkAllReadLogic(authContext(7, "作者甲"), s).MarkAllRead()
	if err == nil {
		t.Fatal("mark all read repo error must propagate")
	}
}
