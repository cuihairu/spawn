package message

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/types"
)

// unreadCountFaultStore 仅 CountUnread 投毒的故障注入仓储（其余方法不可达，
// 嵌入接口即可满足 DirectMessageStore）。
type unreadCountFaultStore struct {
	model.DirectMessageStore
}

func (s unreadCountFaultStore) CountUnread(int64) (int64, error) {
	return 0, errors.New("unread count on fire")
}

// TestMarkDMThreadRead_CountUnreadErrorPropagates 已读统计失败 → 500 显式报错，
// 绝不静默回落 Count:0（否则多端徽标被错误清零），且不广播 message.read。
func TestMarkDMThreadRead_CountUnreadErrorPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	rec := &recordingPublisher{}
	s.Realtime = rec
	seedDM(t, s, 8, 7, "你好")

	s.MessageRepo = unreadCountFaultStore{DirectMessageStore: s.MessageRepo}

	resp, err := NewMarkDMThreadReadLogic(authContext(7, "读者甲"), s).
		MarkDMThreadRead(&types.DMThreadReadReq{PeerId: 8})
	if err == nil {
		t.Fatalf("count failure must propagate, got resp=%+v", resp)
	}
	var appErr *httperr.Error
	if !errors.As(err, &appErr) || appErr.Status != 500 {
		t.Fatalf("err = %v (%T), want 500 httperr", err, err)
	}
	if len(rec.events) != 0 {
		t.Fatalf("failed read must not publish, got %+v", rec.events)
	}
}
