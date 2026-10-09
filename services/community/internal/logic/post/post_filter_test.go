package post

import (
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

// --- 敏感词过滤（阻塞式）：命中即 400，提示首个违规词 ---

func TestCreatePost_BlockedWord(t *testing.T) {
	svcCtx := newTestServiceContext(t)
	l := NewCreatePostLogic(authContext(1001, "demo"), svcCtx)

	// 标题命中
	_, err := l.CreatePost(&types.CreatePostReq{TopicId: 1, Title: "卖外挂", Content: "c"})
	requireHTTPStatus(t, err, 400)

	// 正文命中
	_, err = l.CreatePost(&types.CreatePostReq{TopicId: 1, Title: "t", Content: "这里能找到代练"})
	requireHTTPStatus(t, err, 400)

	// 大小写不敏感（英文词场景），内置中文表不误伤正常文案
	_, err = l.CreatePost(&types.CreatePostReq{TopicId: 1, Title: "正常标题", Content: "正常讨论"})
	if err != nil {
		t.Fatalf("clean post should pass: %v", err)
	}
}

func TestUpdatePost_BlockedWord(t *testing.T) {
	svcCtx := newTestServiceContext(t)
	created, err := NewCreatePostLogic(authContext(1001, "demo"), svcCtx).
		CreatePost(&types.CreatePostReq{TopicId: 1, Title: "t", Content: "c"})
	if err != nil {
		t.Fatalf("seed post: %v", err)
	}
	l := NewUpdatePostLogic(authContext(1001, "demo"), svcCtx)

	// 更新内容命中 → 400，帖子内容保持原值
	_, err = l.UpdatePost(&types.UpdatePostReq{Id: created.Post.Id, Content: "含赌博信息"})
	requireHTTPStatus(t, err, 400)
	p, err := svcCtx.PostRepo.Get(created.Post.Id)
	if err != nil || p.Content != "c" {
		t.Fatalf("rejected update should keep old content: p=%+v err=%v", p, err)
	}

	// 干净字段更新照常成功
	if _, err := l.UpdatePost(&types.UpdatePostReq{Id: created.Post.Id, Title: "新标题"}); err != nil {
		t.Fatalf("clean update: %v", err)
	}
}
