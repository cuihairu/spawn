package comment

import (
	"context"
	"errors"
	"testing"

	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/types"
)

// failingPostStoreForComment 仅 IncrementComments 投毒的帖子仓储，其余方法
// 保持真实实现，覆盖 CreateComment 的计数失败记日志分支。
type failingPostStoreForComment struct {
	model.PostStore
	err error
}

func (s failingPostStoreForComment) IncrementComments(int64) (*types.Post, error) {
	return nil, s.err
}

// failingNotificationStoreForComment 仅 Create 投毒的通知仓储，覆盖通知创建
// 失败不中断评论提交的记日志分支。
type failingNotificationStoreForComment struct {
	model.NotificationStore
	err error
}

func (s failingNotificationStoreForComment) Create(n *types.Notification) error {
	return s.err
}

// TestCreateComment_IncrementCommentsFailureStillSucceeds 计数自增失败不阻
// 止评论提交，仅记日志。
func TestCreateComment_IncrementCommentsFailureStillSucceeds(t *testing.T) {
	s := newTestServiceContext(t)
	s.PostRepo = failingPostStoreForComment{PostStore: s.PostRepo, err: errors.New("counter on fire")}

	postId := hostPost(t, s)
	resp, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).
		CreateComment(&types.CreateCommentReq{Id: postId, Content: "计数坏也能发"})
	if err != nil || resp == nil {
		t.Fatalf("comment=%v err=%v, want ok", resp, err)
	}
}

// TestCreateComment_NotificationFailureStillSucceeds 通知写入失败不阻止评论提交。
func TestCreateComment_NotificationFailureStillSucceeds(t *testing.T) {
	s := newTestServiceContext(t)
	s.NotificationRepo = failingNotificationStoreForComment{NotificationStore: s.NotificationRepo, err: errors.New("notifier on fire")}

	postId := hostPost(t, s)
	resp, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).
		CreateComment(&types.CreateCommentReq{Id: postId, Content: "通知坏也能发"})
	if err != nil || resp == nil {
		t.Fatalf("comment=%v err=%v, want ok", resp, err)
	}
}

// TestListComments_LimitClamp limit>200 应被钳制到 200（请求允许通过即可）。
func TestListComments_LimitClamp(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)

	resp, err := NewListCommentsLogic(context.Background(), s).
		ListComments(&types.ListCommentsReq{Id: postId, Limit: 300})
	if err != nil || resp == nil {
		t.Fatalf("resp=%v err=%v, want ok", resp, err)
	}
}

// TestTruncateRunes 覆盖短字符串不截断、长字符串截断两个分支。
func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		in, want string
		n        int
	}{
		{"short", "short", 100},
		{"hello world", "hello", 5},
		{"中文字符串", "中文字", 3},
	}
	for _, tc := range cases {
		if got := truncateRunes(tc.in, tc.n); got != tc.want {
			t.Fatalf("truncateRunes(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}
