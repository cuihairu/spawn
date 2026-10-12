package message

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/tappi/tappi/services/community/internal/config"
	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
)

// newTestServiceContext 与 notification 包同款：真实服务上下文 + SQLite 临时库。
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

// recordingPublisher 记录 Publish 调用（message 包测试专用）。
type recordingPublisher struct {
	mu     sync.Mutex
	events []recEvent
}

type recEvent struct {
	typ     string
	userIds []int64
	payload any
}

func (p *recordingPublisher) Publish(eventType string, userIds []int64, payload any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, recEvent{eventType, userIds, payload})
}

// seedDM 绕过 logic 直插一条消息（造未读/会话数据用）。
func seedDM(t *testing.T, s *svc.ServiceContext, from, to int64, content string) *types.DirectMessage {
	t.Helper()
	msg := &types.DirectMessage{
		SenderId: from, RecipientId: to, Content: content,
		CreatedAt: "2026-10-12T00:00:00Z",
	}
	if err := s.MessageRepo.Send(msg); err != nil {
		t.Fatalf("seed dm: %v", err)
	}
	return msg
}

// TestSendDirectMessage_PublishesFrame 发送成功落库并向收件人推 message 帧。
func TestSendDirectMessage_PublishesFrame(t *testing.T) {
	s := newTestServiceContext(t)
	rec := &recordingPublisher{}
	s.Realtime = rec

	resp, err := NewSendDirectMessageLogic(authContext(7, "作者甲"), s).
		SendDirectMessage(&types.SendDirectMessageReq{RecipientId: 8, Content: "  你好呀  "})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.Message.Id <= 0 || resp.Message.Content != "你好呀" || resp.Message.SenderId != 7 || resp.Message.RecipientId != 8 {
		t.Fatalf("message mismatch: %+v", resp.Message)
	}
	if utf8.RuneCountInString(resp.Message.CreatedAt) == 0 {
		t.Fatalf("created_at empty")
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 1 || rec.events[0].typ != "message" {
		t.Fatalf("events = %+v", rec.events)
	}
	if len(rec.events[0].userIds) != 1 || rec.events[0].userIds[0] != 8 {
		t.Fatalf("user_ids = %v", rec.events[0].userIds)
	}
	payload, ok := rec.events[0].payload.(map[string]any)
	if !ok || payload["from_id"] != int64(7) || payload["from_name"] != "作者甲" ||
		payload["content"] != "你好呀" || payload["message_id"] != resp.Message.Id {
		t.Fatalf("payload = %#v", rec.events[0].payload)
	}
}

// TestSendDirectMessage_Validations 非法入参 400 且不落库不发布。
func TestSendDirectMessage_Validations(t *testing.T) {
	s := newTestServiceContext(t)
	rec := &recordingPublisher{}
	s.Realtime = rec
	l := NewSendDirectMessageLogic(authContext(7, "作者甲"), s)

	cases := []struct {
		name string
		req  types.SendDirectMessageReq
	}{
		{"self", types.SendDirectMessageReq{RecipientId: 7, Content: "自言自语"}},
		{"empty", types.SendDirectMessageReq{RecipientId: 8, Content: "   "}},
		{"long", types.SendDirectMessageReq{RecipientId: 8, Content: strings.Repeat("字", 2001)}},
		{"badRecipient", types.SendDirectMessageReq{RecipientId: 0, Content: "hi"}},
	}
	for _, tc := range cases {
		if _, err := l.SendDirectMessage(&tc.req); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		} else if he, ok := err.(*httperr.Error); !ok || he.Status != 400 {
			t.Fatalf("%s: want 400, got %v", tc.name, err)
		}
	}

	var total int64
	list, total, err := s.MessageRepo.Thread(7, 8, 50, 0)
	if err != nil || total != 0 || len(list) != 0 {
		t.Fatalf("nothing should be stored: total=%d err=%v", total, err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 0 {
		t.Fatalf("unexpected events = %+v", rec.events)
	}
}

// TestMarkDMThreadRead_PublishesReceiptWithCount 会话置已读回执带剩余未读数。
func TestMarkDMThreadRead_PublishesReceiptWithCount(t *testing.T) {
	s := newTestServiceContext(t)
	rec := &recordingPublisher{}
	s.Realtime = rec
	seedDM(t, s, 8, 7, "a")
	seedDM(t, s, 8, 7, "b")
	seedDM(t, s, 9, 7, "c")

	resp, err := NewMarkDMThreadReadLogic(authContext(7, "作者甲"), s).
		MarkDMThreadRead(&types.DMThreadReadReq{PeerId: 8})
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("remaining count = %d, want 1", resp.Count)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 1 || rec.events[0].typ != "message.read" ||
		len(rec.events[0].userIds) != 1 || rec.events[0].userIds[0] != 7 {
		t.Fatalf("events = %+v", rec.events)
	}
	payload := rec.events[0].payload.(map[string]any)
	if payload["unread_count"] != int64(1) || payload["peer_id"] != int64(8) {
		t.Fatalf("payload = %#v", payload)
	}
}

// TestDMQueries_ListAndUnread 会话列表 / 线程 / 未读数经 logic 层贯通。
func TestDMQueries_ListAndUnread(t *testing.T) {
	s := newTestServiceContext(t)
	seedDM(t, s, 8, 7, "hello")
	seedDM(t, s, 7, 8, "hi")

	thread, err := NewGetDMThreadLogic(authContext(7, "作者甲"), s).
		GetDMThread(&types.DMThreadReq{PeerId: 8, Limit: -1, Offset: -3})
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if thread.Total != 2 || len(thread.Messages) != 2 || thread.Messages[0].Content != "hi" {
		t.Fatalf("thread = %+v", thread)
	}

	unread, err := NewGetDMUnreadCountLogic(authContext(7, "作者甲"), s).GetDMUnreadCount()
	if err != nil || unread.Count != 1 {
		t.Fatalf("unread = %+v err=%v", unread, err)
	}

	convs, err := NewListDMConversationsLogic(authContext(7, "作者甲"), s).ListDMConversations()
	if err != nil || len(convs.Conversations) != 1 || convs.Conversations[0].PeerId != 8 ||
		convs.Conversations[0].Unread != 1 || convs.Conversations[0].LastMessage != "hi" {
		t.Fatalf("convs = %+v err=%v", convs, err)
	}

	// 越权路径隔离：9 的线程只有自己与 8/7 无关
	peerThread, err := NewGetDMThreadLogic(authContext(9, "路人丙"), s).
		GetDMThread(&types.DMThreadReq{PeerId: 8})
	if err != nil || peerThread.Total != 0 {
		t.Fatalf("peer thread should be empty: %+v err=%v", peerThread, err)
	}

	// peer_id 缺失 400
	if _, err := NewGetDMThreadLogic(authContext(7, "作者甲"), s).
		GetDMThread(&types.DMThreadReq{}); err == nil {
		t.Fatal("expected 400 for missing peer")
	}
}
