package comment

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

// TestReplyCommentPublishesBothEvents 回复场景推两帧：帖子作者收
// comment_post、被回复者收 reply_comment（作者与被回复者不同人）。
func TestReplyCommentPublishesBothEvents(t *testing.T) {
	s := newTestServiceContext(t)
	rec := &recordingPublisher{}
	s.Realtime = rec
	postId := hostPost(t, s) // 作者 7

	// 用户 9 在帖下留根评论
	root, err := NewCreateCommentLogic(authContext(9, "读者丙"), s).CreateComment(&types.CreateCommentReq{
		Id: postId, Content: "根评论",
	})
	if err != nil {
		t.Fatalf("root comment: %v", err)
	}
	rec.events = nil // 清掉根评论自己的 comment_post

	// 用户 8 回复用户 9 的评论
	if _, err := NewCreateCommentLogic(authContext(8, "读者乙"), s).CreateComment(&types.CreateCommentReq{
		Id: postId, Content: "回复内容", ParentId: root.Comment.Id,
	}); err != nil {
		t.Fatalf("reply: %v", err)
	}

	want := []string{"[7] notification comment_post", "[9] notification reply_comment"}
	if len(rec.events) != len(want) {
		t.Fatalf("events = %v, want %v", rec.events, want)
	}
	for i := range want {
		if rec.events[i] != want[i] {
			t.Fatalf("event[%d] = %q, want %q", i, rec.events[i], want[i])
		}
	}
}
