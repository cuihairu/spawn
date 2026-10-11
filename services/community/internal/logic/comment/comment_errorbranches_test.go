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

// failingCommentCreateStore 仅 Create 投毒的评论仓储（其余方法保持真实实现），
// 覆盖评论写库失败原样透传分支。
type failingCommentCreateStore struct {
	model.CommentStore
	err error
}

func (s failingCommentCreateStore) Create(*types.Comment) (int64, error) { return 0, s.err }

// failingCommentListStore 仅 ListByPost 投毒。
type failingCommentListStore struct {
	model.CommentStore
	err error
}

func (s failingCommentListStore) ListByPost(int64, int64, int64) ([]*types.Comment, error) {
	return nil, s.err
}

// failingCommentCountStore 仅 CountByPost 投毒（列表成功、计数单独失败）。
type failingCommentCountStore struct {
	model.CommentStore
	err error
}

func (s failingCommentCountStore) CountByPost(int64) (int64, error) { return 0, s.err }

// failingCommentDeleteStore 仅 Delete 投毒（作者校验通过后写删失败）。
type failingCommentDeleteStore struct {
	model.CommentStore
	err error
}

func (s failingCommentDeleteStore) Delete(int64) error { return s.err }

// TestCreateComment_CreateFailsPropagates 评论写库失败 → 错误原样透传。
func TestCreateComment_CreateFailsPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)
	s.CommentRepo = failingCommentCreateStore{CommentStore: s.CommentRepo, err: errors.New("comment write on fire")}

	if _, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).
		CreateComment(&types.CreateCommentReq{Id: postId, Content: "写库必败"}); err == nil {
		t.Fatal("comment create failure must propagate")
	}
}

// TestCreateComment_ReplyNotificationFailureStillSucceeds 回复通知写入失败
// 不阻止回复提交（comment_post 与 reply_comment 双双记日志，响应仍 ok）。
func TestCreateComment_ReplyNotificationFailureStillSucceeds(t *testing.T) {
	s := newTestServiceContext(t)
	s.NotificationRepo = failingNotificationStoreForComment{NotificationStore: s.NotificationRepo, err: errors.New("notifier on fire")}

	postId := hostPost(t, s) // 作者 7
	first, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{Id: postId, Content: "首评"})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	// 用户 9 回复用户 8 的评论：被回复者既非自己也非帖子作者，双通知都走失败日志
	resp, err := NewCreateCommentLogic(authContext(9, "路人丙"), s).CreateComment(&types.CreateCommentReq{
		Id: postId, Content: "回复首评", ParentId: first.Comment.Id,
	})
	if err != nil || resp == nil {
		t.Fatalf("reply=%v err=%v, want ok", resp, err)
	}
}

// TestListComments_OffsetClamped 负 offset 钳到 0。
func TestListComments_OffsetClamped(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)

	resp, err := NewListCommentsLogic(authContext(8, "读者乙"), s).
		ListComments(&types.ListCommentsReq{Id: postId, Offset: -5})
	if err != nil || resp == nil {
		t.Fatalf("list=%v err=%v, want ok with negative offset", resp, err)
	}
}

// TestListComments_ListByPostErrorPropagates 列表查询失败 → 错误透传。
func TestListComments_ListByPostErrorPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)
	s.CommentRepo = failingCommentListStore{CommentStore: s.CommentRepo, err: errors.New("list on fire")}

	if _, err := NewListCommentsLogic(authContext(8, "读者乙"), s).
		ListComments(&types.ListCommentsReq{Id: postId}); err == nil {
		t.Fatal("list failure must propagate")
	}
}

// TestListComments_CountByPostErrorPropagates 计数单独失败 → 错误透传。
func TestListComments_CountByPostErrorPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)
	s.CommentRepo = failingCommentCountStore{CommentStore: s.CommentRepo, err: errors.New("count on fire")}

	if _, err := NewListCommentsLogic(authContext(8, "读者乙"), s).
		ListComments(&types.ListCommentsReq{Id: postId}); err == nil {
		t.Fatal("count failure must propagate")
	}
}

// TestDeleteComment_MissingCommentId 缺评论 id → 400 信封。
func TestDeleteComment_MissingCommentId(t *testing.T) {
	s := newTestServiceContext(t)
	_, err := NewDeleteCommentLogic(authContext(8, "读者乙"), s).
		DeleteComment(&types.DeleteCommentReq{Id: 1, CommentId: 0})
	requireHTTPStatus(t, err, 400)
}

// TestDeleteComment_DeleteFailsPropagates 作者校验通过后写删失败 → 错误透传。
func TestDeleteComment_DeleteFailsPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)
	first, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{Id: postId, Content: "待删"})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	s.CommentRepo = failingCommentDeleteStore{CommentStore: s.CommentRepo, err: errors.New("delete on fire")}

	if _, err := NewDeleteCommentLogic(authContext(8, "读者乙"), s).
		DeleteComment(&types.DeleteCommentReq{Id: postId, CommentId: first.Comment.Id}); err == nil {
		t.Fatal("delete failure must propagate")
	}
}
