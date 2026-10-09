package comment

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/community/internal/config"
	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
)

// newTestServiceContext 与 post 包同款：真实服务上下文 + SQLite 临时库。
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

func requireHTTPStatus(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want HTTP %d", want)
	}
	var appErr *httperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("err = %v (%T), want *httperr.Error with status %d", err, err, want)
	}
	if appErr.Status != want {
		t.Fatalf("status = %d, want %d (message %q)", appErr.Status, want, appErr.Message)
	}
}

// hostPost 建一条宿主帖子并返回 id（作者 id=7「作者甲」）。
func hostPost(t *testing.T, s *svc.ServiceContext) int64 {
	t.Helper()
	p, err := s.PostRepo.Create(1, 7, "作者甲", &types.CreatePostReq{Title: "宿主", Content: "正文"})
	if err != nil {
		t.Fatalf("create host post: %v", err)
	}
	return p.Id
}

func TestCreateComment(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)

	l := NewCreateCommentLogic(authContext(8, "读者乙"), s)
	resp, err := l.CreateComment(&types.CreateCommentReq{Id: postId, Content: "  好帖  "})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 正文去首尾空白后落库
	if resp.Comment.Content != "好帖" || resp.Comment.AuthorId != 8 || resp.Comment.AuthorName != "读者乙" {
		t.Fatalf("comment mismatch: %+v", resp.Comment)
	}
	if resp.Comment.PostId != postId || resp.Comment.ParentId != 0 {
		t.Fatalf("post/parent mismatch: %+v", resp.Comment)
	}

	// 帖子评论计数联动 +1
	p, err := s.PostRepo.Get(postId)
	if err != nil || p.CommentCount != 1 {
		t.Fatalf("comment_count=%d err=%v, want 1", p.CommentCount, err)
	}
}

func TestCreateCommentReply(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)

	first, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{Id: postId, Content: "第一条"})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := NewCreateCommentLogic(authContext(7, "作者甲"), s).CreateComment(&types.CreateCommentReq{
		Id: postId, Content: "回复第一条", ParentId: first.Comment.Id,
	})
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}
	// 被回复者昵称由服务端从父评论冗余
	if second.Comment.ParentId != first.Comment.Id || second.Comment.ReplyToAuthorName != "读者乙" {
		t.Fatalf("reply mismatch: %+v", second.Comment)
	}
}

func TestCreateCommentValidation(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)

	cases := []struct {
		name string
		ctx  context.Context
		req  *types.CreateCommentReq
		want int
	}{
		{"缺登录", context.Background(), &types.CreateCommentReq{Id: postId, Content: "x"}, 401},
		{"空正文", authContext(8, "读者乙"), &types.CreateCommentReq{Id: postId, Content: "   "}, 400},
		{"缺帖子id", authContext(8, "读者乙"), &types.CreateCommentReq{Id: 0, Content: "x"}, 400},
		{"nil 请求", authContext(8, "读者乙"), nil, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewCreateCommentLogic(tc.ctx, s).CreateComment(tc.req)
			requireHTTPStatus(t, err, tc.want)
		})
	}

	// 帖子/父评论不存在走模型哨兵错误（HTTP 层由 ErrorHandler 映射 404）。
	if _, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{Id: 424242, Content: "x"}); !errors.Is(err, model.ErrPostNotFound) {
		t.Fatalf("missing post err=%v, want ErrPostNotFound", err)
	}
	if _, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{Id: postId, Content: "x", ParentId: 999}); !errors.Is(err, model.ErrCommentNotFound) {
		t.Fatalf("missing parent err=%v, want ErrCommentNotFound", err)
	}
}

func TestCreateCommentParentCrossPost(t *testing.T) {
	s := newTestServiceContext(t)
	postA := hostPost(t, s)
	postB, err := s.PostRepo.Create(1, 7, "作者甲", &types.CreatePostReq{Title: "另一帖", Content: "正文"})
	if err != nil {
		t.Fatalf("create post B: %v", err)
	}

	// 父评论挂在 postA，却在 postB 下回复 → 400
	parent, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{Id: postA, Content: "A 下的评论"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	_, err = NewCreateCommentLogic(authContext(7, "作者甲"), s).CreateComment(&types.CreateCommentReq{
		Id: postB.Id, Content: "跨帖回复", ParentId: parent.Comment.Id,
	})
	requireHTTPStatus(t, err, 400)
}

func TestListComments(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)

	for i := 0; i < 3; i++ {
		if _, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{
			Id: postId, Content: "评论" + string(rune('A'+i)),
		}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	l := NewListCommentsLogic(context.Background(), s)
	resp, err := l.ListComments(&types.ListCommentsReq{Id: postId})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.Total != 3 || len(resp.Comments) != 3 {
		t.Fatalf("total=%d n=%d, want 3/3", resp.Total, len(resp.Comments))
	}
	// id 升序
	if resp.Comments[0].Id > resp.Comments[1].Id || resp.Comments[1].Id > resp.Comments[2].Id {
		t.Fatalf("not ascending: %+v", resp.Comments)
	}

	// 分页钳制：limit<=0 → 50 语义下取全量；limit>200 → 200；offset<0 → 0。
	page, err := l.ListComments(&types.ListCommentsReq{Id: postId, Limit: 2, Offset: 1})
	if err != nil || len(page.Comments) != 2 {
		t.Fatalf("paged n=%d err=%v, want 2", len(page.Comments), err)
	}

	// 帖子不存在走模型哨兵错误（HTTP 层映射 404）；缺 id → 400
	_, err = l.ListComments(&types.ListCommentsReq{Id: 424242})
	if !errors.Is(err, model.ErrPostNotFound) {
		t.Fatalf("missing post err=%v, want ErrPostNotFound", err)
	}
	_, err = l.ListComments(nil)
	requireHTTPStatus(t, err, 400)
}

func TestDeleteComment(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)

	created, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{Id: postId, Content: "待删"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	cid := created.Comment.Id

	// 非作者删除 → 403
	_, err = NewDeleteCommentLogic(authContext(7, "作者甲"), s).DeleteComment(&types.DeleteCommentReq{Id: postId, CommentId: cid})
	requireHTTPStatus(t, err, 403)

	// cid 不属于路径里的帖子 → 400
	other := hostPost(t, s)
	_, err = NewDeleteCommentLogic(authContext(8, "读者乙"), s).DeleteComment(&types.DeleteCommentReq{Id: other, CommentId: cid})
	requireHTTPStatus(t, err, 400)

	// 作者本人删除 → ok
	resp, err := NewDeleteCommentLogic(authContext(8, "读者乙"), s).DeleteComment(&types.DeleteCommentReq{Id: postId, CommentId: cid})
	if err != nil || resp.Message != "ok" {
		t.Fatalf("delete: resp=%+v err=%v", resp, err)
	}

	// 删除后列表不再返回；comment_count 不回退（单调累加契约）
	list, err := NewListCommentsLogic(context.Background(), s).ListComments(&types.ListCommentsReq{Id: postId})
	if err != nil || list.Total != 0 {
		t.Fatalf("after delete total=%d err=%v, want 0", list.Total, err)
	}
	p, _ := s.PostRepo.Get(postId)
	if p.CommentCount != 1 {
		t.Fatalf("comment_count=%d, want 1 (monotonic)", p.CommentCount)
	}

	// 缺登录 → 401；不存在走模型哨兵错误（HTTP 层映射 404）
	_, err = NewDeleteCommentLogic(context.Background(), s).DeleteComment(&types.DeleteCommentReq{Id: postId, CommentId: cid})
	requireHTTPStatus(t, err, 401)
	_, err = NewDeleteCommentLogic(authContext(8, "读者乙"), s).DeleteComment(&types.DeleteCommentReq{Id: postId, CommentId: 9999})
	if !errors.Is(err, model.ErrCommentNotFound) {
		t.Fatalf("missing comment err=%v, want ErrCommentNotFound", err)
	}
}

// listNotificationsOf 读某收件人的全部通知（触发点测试断言用）。
func listNotificationsOf(t *testing.T, s *svc.ServiceContext, userId int64) []*types.Notification {
	t.Helper()
	list, _, err := s.NotificationRepo.ListByUser(userId, 50, 0)
	if err != nil {
		t.Fatalf("list notifications for %d: %v", userId, err)
	}
	return list
}

func TestCreateCommentNotifiesPostAuthor(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s) // 作者 7「作者甲」

	if _, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{
		Id: postId, Content: "写得真好",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	list := listNotificationsOf(t, s, 7)
	if len(list) != 1 {
		t.Fatalf("author notifications=%d, want 1", len(list))
	}
	if list[0].Type != "comment_post" || list[0].ActorId != 8 || list[0].TargetId != postId {
		t.Fatalf("notification mismatch: %+v", list[0])
	}
	// 评论者自己与第三方均无通知
	if got := listNotificationsOf(t, s, 8); len(got) != 0 {
		t.Fatalf("commenter should not be notified: %+v", got)
	}
}

func TestCreateCommentNoSelfNotification(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s) // 作者 7

	if _, err := NewCreateCommentLogic(authContext(7, "作者甲"), s).CreateComment(&types.CreateCommentReq{
		Id: postId, Content: "自评",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := listNotificationsOf(t, s, 7); len(got) != 0 {
		t.Fatalf("self-comment should not notify: %+v", got)
	}
}

func TestCreateCommentReplyNotifiesBothParties(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s) // 帖子作者 7

	parent, err := NewCreateCommentLogic(authContext(9, "读者丙"), s).CreateComment(&types.CreateCommentReq{
		Id: postId, Content: "第一条评论",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	// 读者乙（8）回复读者丙（9）：帖子作者收到两条 comment_post（不同 actor 各一条），
	// 被回复者收到 reply_comment
	if _, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{
		Id: postId, Content: "回复你", ParentId: parent.Comment.Id,
	}); err != nil {
		t.Fatalf("create reply: %v", err)
	}

	authorList := listNotificationsOf(t, s, 7)
	if len(authorList) != 2 {
		t.Fatalf("author notifications=%d, want 2 comment_post (one per actor)", len(authorList))
	}
	for _, n := range authorList {
		if n.Type != "comment_post" {
			t.Fatalf("author should only get comment_post: %+v", authorList)
		}
	}

	parentList := listNotificationsOf(t, s, 9)
	if len(parentList) != 1 || parentList[0].Type != "reply_comment" {
		t.Fatalf("parent notifications=%+v, want 1 reply_comment", parentList)
	}
}

func TestCreateCommentReplyToPostAuthorSingleNotification(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s) // 帖子作者 7

	// 帖子作者先评论，读者乙回复帖子作者 → 被回复者即帖子作者，只发一条 comment_post
	parent, err := NewCreateCommentLogic(authContext(7, "作者甲"), s).CreateComment(&types.CreateCommentReq{
		Id: postId, Content: "作者自评",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if _, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{
		Id: postId, Content: "回复作者", ParentId: parent.Comment.Id,
	}); err != nil {
		t.Fatalf("create reply: %v", err)
	}

	list := listNotificationsOf(t, s, 7)
	if len(list) != 1 || list[0].Type != "comment_post" {
		t.Fatalf("author notifications=%+v, want exactly 1 comment_post", list)
	}
}
