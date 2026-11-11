package ticket

import (
	"errors"
	"testing"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/courier"
)

func TestIssueRedeemOneTime(t *testing.T) {
	s := NewStore(time.Minute)
	sess := &courier.Session{AccessToken: "atk", AccountID: "acc"}
	tk, err := s.Issue(77, "cg1", "prod", sess)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if len(tk.Token) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars", len(tk.Token))
	}
	got, err := s.Redeem(tk.Token)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if got.Session != sess || got.CourierGameID != "cg1" || got.SpawnUserID != 77 {
		t.Fatalf("ticket payload mismatch: %+v", got)
	}
	if _, err := s.Redeem(tk.Token); !errors.Is(err, ErrTicketNotFound) {
		t.Fatalf("second redeem err = %v, want ErrTicketNotFound (one-time)", err)
	}
	if _, err := s.Redeem("deadbeef"); !errors.Is(err, ErrTicketNotFound) {
		t.Fatalf("unknown token err = %v", err)
	}
}

func TestTicketExpiry(t *testing.T) {
	s := NewStore(10 * time.Millisecond)
	tk, err := s.Issue(1, "cg", "dev", &courier.Session{})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := s.Redeem(tk.Token); !errors.Is(err, ErrTicketNotFound) {
		t.Fatalf("expired redeem err = %v, want ErrTicketNotFound", err)
	}
}

func TestIssueSweepsExpired(t *testing.T) {
	// P1-1 回归：过期未兑票据在下次签发时被清扫（原实现永久驻留 map）。
	s := NewStore(10 * time.Millisecond)
	for i := 0; i < 3; i++ {
		if _, err := s.Issue(int64(i), "cg", "dev", &courier.Session{}); err != nil {
			t.Fatalf("issue[%d]: %v", i, err)
		}
	}
	if len(s.items) != 3 {
		t.Fatalf("items after 3 issues = %d, want 3", len(s.items))
	}
	time.Sleep(30 * time.Millisecond)
	// 签发第 4 张触发惰性清扫：3 张过期全清，仅新票留存。
	if _, err := s.Issue(9, "cg", "dev", &courier.Session{}); err != nil {
		t.Fatalf("issue trigger: %v", err)
	}
	if len(s.items) != 1 {
		t.Fatalf("items after sweep = %d, want 1（过期票据应被清扫）", len(s.items))
	}
}
