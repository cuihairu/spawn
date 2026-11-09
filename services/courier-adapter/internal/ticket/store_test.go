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
