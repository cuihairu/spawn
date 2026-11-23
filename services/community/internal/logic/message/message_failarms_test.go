package message

// 私信 logic 失败臂收口：未登录上下文 401（五处 logic）、仓储各方法 DB
// 错误原样透传、已读 peer 缺失 400、线程 limit>200 钳制到 200。仓储失败
// 用嵌入接口的定向故障注入（同 unreadCountFaultStore 模式），钳制用
// spy 仓储捕获透传参数。

import (
	"context"
	"errors"
	"testing"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/types"
)

// faultStore 按 failOn 定向投毒的故障仓储（其余方法透传真实仓储）。
type faultStore struct {
	model.DirectMessageStore
	failOn string
}

func (s faultStore) Send(msg *types.DirectMessage) error {
	if s.failOn == "Send" {
		return errors.New("send on fire")
	}
	return s.DirectMessageStore.Send(msg)
}

func (s faultStore) Conversations(userId int64) ([]*types.DMConversation, error) {
	if s.failOn == "Conversations" {
		return nil, errors.New("conversations on fire")
	}
	return s.DirectMessageStore.Conversations(userId)
}

func (s faultStore) Thread(userId, peer, limit, offset int64) ([]*types.DirectMessage, int64, error) {
	if s.failOn == "Thread" {
		return nil, 0, errors.New("thread on fire")
	}
	return s.DirectMessageStore.Thread(userId, peer, limit, offset)
}

func (s faultStore) MarkPeerRead(userId, peer int64) (int64, error) {
	if s.failOn == "MarkPeerRead" {
		return 0, errors.New("mark peer read on fire")
	}
	return s.DirectMessageStore.MarkPeerRead(userId, peer)
}

func (s faultStore) CountUnread(userId int64) (int64, error) {
	if s.failOn == "CountUnread" {
		return 0, errors.New("count unread on fire")
	}
	return s.DirectMessageStore.CountUnread(userId)
}

// threadSpyStore 捕获 Thread 收到的 limit（验证钳制）。
type threadSpyStore struct {
	model.DirectMessageStore
	gotLimit int64
}

func (s *threadSpyStore) Thread(userId, peer, limit, offset int64) ([]*types.DirectMessage, int64, error) {
	s.gotLimit = limit
	return s.DirectMessageStore.Thread(userId, peer, limit, offset)
}

// wantUnauthorized 断言 401 httperr。
func wantUnauthorized(t *testing.T, name string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: want unauthorized", name)
	}
	var appErr *httperr.Error
	if !errors.As(err, &appErr) || appErr.Status != 401 {
		t.Fatalf("%s: err = %v (%T), want 401 httperr", name, err, err)
	}
}

// TestDMLogics_Unauthenticated 未登录上下文：五处 logic 一律 401，不触仓储。
func TestDMLogics_Unauthenticated(t *testing.T) {
	s := newTestServiceContext(t)
	anonymous := context.Background()

	_, err := NewSendDirectMessageLogic(anonymous, s).
		SendDirectMessage(&types.SendDirectMessageReq{RecipientId: 8, Content: "hi"})
	wantUnauthorized(t, "send", err)

	_, err = NewGetDMThreadLogic(anonymous, s).GetDMThread(&types.DMThreadReq{PeerId: 8})
	wantUnauthorized(t, "thread", err)

	_, err = NewGetDMUnreadCountLogic(anonymous, s).GetDMUnreadCount()
	wantUnauthorized(t, "unread", err)

	_, err = NewMarkDMThreadReadLogic(anonymous, s).MarkDMThreadRead(&types.DMThreadReadReq{PeerId: 8})
	wantUnauthorized(t, "mark read", err)

	_, err = NewListDMConversationsLogic(anonymous, s).ListDMConversations()
	wantUnauthorized(t, "conversations", err)
}

// TestSendDirectMessage_SendError 仓储写失败原样透传，不发布帧。
func TestSendDirectMessage_SendError(t *testing.T) {
	s := newTestServiceContext(t)
	rec := &recordingPublisher{}
	s.Realtime = rec
	s.MessageRepo = faultStore{DirectMessageStore: s.MessageRepo, failOn: "Send"}

	if _, err := NewSendDirectMessageLogic(authContext(7, "作者甲"), s).
		SendDirectMessage(&types.SendDirectMessageReq{RecipientId: 8, Content: "hi"}); err == nil {
		t.Fatal("send db error must propagate")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 0 {
		t.Fatalf("failed send must not publish, got %+v", rec.events)
	}
}

// TestGetDMThread_LimitClamped limit>200 钳制到 200（与帖子列表同口径）。
func TestGetDMThread_LimitClamped(t *testing.T) {
	s := newTestServiceContext(t)
	spy := &threadSpyStore{DirectMessageStore: s.MessageRepo}
	s.MessageRepo = spy

	if _, err := NewGetDMThreadLogic(authContext(7, "作者甲"), s).
		GetDMThread(&types.DMThreadReq{PeerId: 8, Limit: 500}); err != nil {
		t.Fatalf("thread: %v", err)
	}
	if spy.gotLimit != 200 {
		t.Fatalf("limit passed to repo = %d, want 200", spy.gotLimit)
	}
}

// TestDMLogics_RepoErrors 其余仓储方法失败原样透传。
func TestDMLogics_RepoErrors(t *testing.T) {
	s := newTestServiceContext(t)
	seedDM(t, s, 8, 7, "hi")
	raw := s.MessageRepo

	s.MessageRepo = faultStore{DirectMessageStore: raw, failOn: "Thread"}
	if _, err := NewGetDMThreadLogic(authContext(7, "作者甲"), s).
		GetDMThread(&types.DMThreadReq{PeerId: 8}); err == nil {
		t.Fatal("thread db error must propagate")
	}

	s.MessageRepo = faultStore{DirectMessageStore: raw, failOn: "Conversations"}
	if _, err := NewListDMConversationsLogic(authContext(7, "作者甲"), s).
		ListDMConversations(); err == nil {
		t.Fatal("conversations db error must propagate")
	}

	s.MessageRepo = faultStore{DirectMessageStore: raw, failOn: "MarkPeerRead"}
	if _, err := NewMarkDMThreadReadLogic(authContext(7, "读者甲"), s).
		MarkDMThreadRead(&types.DMThreadReadReq{PeerId: 8}); err == nil {
		t.Fatal("mark peer read db error must propagate")
	}

	s.MessageRepo = faultStore{DirectMessageStore: raw, failOn: "CountUnread"}
	if _, err := NewGetDMUnreadCountLogic(authContext(7, "作者甲"), s).
		GetDMUnreadCount(); err == nil {
		t.Fatal("count unread db error must propagate")
	}
}

// TestMarkDMThreadRead_MissingPeer peer 缺失 400。
func TestMarkDMThreadRead_MissingPeer(t *testing.T) {
	s := newTestServiceContext(t)
	if _, err := NewMarkDMThreadReadLogic(authContext(7, "作者甲"), s).
		MarkDMThreadRead(&types.DMThreadReadReq{}); err == nil {
		t.Fatal("want 400 for missing peer")
	}
}
