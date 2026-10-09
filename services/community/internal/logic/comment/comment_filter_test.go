package comment

import (
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

// --- 敏感词过滤（阻塞式）：命中即 400 ---

func TestCreateComment_BlockedWord(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)
	l := NewCreateCommentLogic(authContext(8, "读者乙"), s)

	// 正文命中 → 400
	_, err := l.CreateComment(&types.CreateCommentReq{Id: postId, Content: "加微信送外挂"})
	requireHTTPStatus(t, err, 400)

	// 干净评论照常成功
	if _, err := l.CreateComment(&types.CreateCommentReq{Id: postId, Content: "正常评论"}); err != nil {
		t.Fatalf("clean comment should pass: %v", err)
	}
}
