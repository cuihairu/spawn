package post

import (
	"testing"

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
