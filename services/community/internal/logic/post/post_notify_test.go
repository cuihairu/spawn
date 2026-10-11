package post

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
)

// listPostNotificationsOf 读某收件人的全部通知（触发点测试断言用）。
func listPostNotificationsOf(t *testing.T, s *svc.ServiceContext, userId int64) []*types.Notification {
	t.Helper()
	list, _, err := s.NotificationRepo.ListByUser(userId, 50, 0)
	if err != nil {
		t.Fatalf("list notifications for %d: %v", userId, err)
	}
	return list
}

func TestLikePostNotifiesAuthor(t *testing.T) {
	s := newTestServiceContext(t)
	authorId := int64(7)
	p, err := s.PostRepo.Create(1, authorId, "作者甲", &types.CreatePostReq{Title: "好帖", Content: "正文"})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	if _, err := NewLikePostLogic(authContext(8, "读者乙"), s).LikePost(&types.LikePostReq{Id: p.Id}); err != nil {
		t.Fatalf("like: %v", err)
	}

	list := listPostNotificationsOf(t, s, authorId)
	if len(list) != 1 {
		t.Fatalf("author notifications=%d, want 1", len(list))
	}
	if list[0].Type != "like_post" || list[0].ActorId != 8 || list[0].TargetId != p.Id {
		t.Fatalf("notification mismatch: %+v", list[0])
	}
	if list[0].Content == "" {
		t.Fatalf("content should carry post title summary")
	}
}

func TestLikePostNoSelfNotification(t *testing.T) {
	s := newTestServiceContext(t)
	authorId := int64(7)
	p, err := s.PostRepo.Create(1, authorId, "作者甲", &types.CreatePostReq{Title: "自赞帖", Content: "正文"})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	if _, err := NewLikePostLogic(authContext(authorId, "作者甲"), s).LikePost(&types.LikePostReq{Id: p.Id}); err != nil {
		t.Fatalf("like: %v", err)
	}
	if got := listPostNotificationsOf(t, s, authorId); len(got) != 0 {
		t.Fatalf("self-like should not notify: %+v", got)
	}
}

func TestLikePostNotificationDeduped(t *testing.T) {
	s := newTestServiceContext(t)
	authorId := int64(7)
	p, err := s.PostRepo.Create(1, authorId, "作者甲", &types.CreatePostReq{Title: "重复赞", Content: "正文"})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	// 同一用户重复点赞：计数非幂等但通知唯一键去重，不刷屏
	for i := 0; i < 2; i++ {
		if _, err := NewLikePostLogic(authContext(8, "读者乙"), s).LikePost(&types.LikePostReq{Id: p.Id}); err != nil {
			t.Fatalf("like #%d: %v", i+1, err)
		}
	}

	list := listPostNotificationsOf(t, s, authorId)
	if len(list) != 1 {
		t.Fatalf("author notifications=%d, want 1 (unique key dedup)", len(list))
	}
}

// failingNotificationStoreForPost 仅 Create 投毒的通知仓储，覆盖点赞通知
// 写失败不中断点赞的记日志分支。
type failingNotificationStoreForPost struct {
	model.NotificationStore
	err error
}

func (s failingNotificationStoreForPost) Create(n *types.Notification) error { return s.err }

// TestLikePostNotificationFailureStillSucceeds 通知写失败不阻止点赞本体。
func TestLikePostNotificationFailureStillSucceeds(t *testing.T) {
	s := newTestServiceContext(t)
	p, err := s.PostRepo.Create(1, 7, "作者甲", &types.CreatePostReq{Title: "好帖", Content: "正文"})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	s.NotificationRepo = failingNotificationStoreForPost{NotificationStore: s.NotificationRepo, err: errors.New("notifier on fire")}

	if _, err := NewLikePostLogic(authContext(8, "读者乙"), s).LikePost(&types.LikePostReq{Id: p.Id}); err != nil {
		t.Fatalf("like must still succeed: %v", err)
	}
}

// TestTruncateRunesTruncates 通知摘要按 rune 截断（中文按字符不按字节），
// 短串原样透传。
func TestTruncateRunesTruncates(t *testing.T) {
	if got := truncateRunes("一二三四五", 3); got != "一二三" {
		t.Fatalf("truncateRunes cjk = %q, want 一二三", got)
	}
	if got := truncateRunes("abcdef", 2); got != "ab" {
		t.Fatalf("truncateRunes ascii = %q, want ab", got)
	}
	if got := truncateRunes("短", 10); got != "短" {
		t.Fatalf("truncateRunes passthrough = %q, want 短", got)
	}
}
