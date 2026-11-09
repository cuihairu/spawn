// Package ticket 游戏启动票据：短期、一次性、绑定 game_id（接入设计
// §1.3 步骤 5）。票据换发 Courier scope-bound token 后即销毁；
// Courier token 明文只在内存短暂持有，不落盘（重启即失效，可接受）。
package ticket

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/courier"
)

var ErrTicketNotFound = errors.New("ticket not found or already redeemed")

// Ticket 一张待兑换的启动票据。
type Ticket struct {
	Token         string
	SpawnUserID   int64
	CourierGameID string
	Env           string
	Session       *courier.Session
	ExpiresAt     time.Time
}

type Store struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]*Ticket
}

func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 120 * time.Second
	}
	return &Store{ttl: ttl, items: make(map[string]*Ticket)}
}

// Issue 签发一张票据（32B 随机 hex）。
func (s *Store) Issue(spawnUserID int64, courierGameID, env string, sess *courier.Session) (*Ticket, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	t := &Ticket{
		Token:         hex.EncodeToString(buf),
		SpawnUserID:   spawnUserID,
		CourierGameID: courierGameID,
		Env:           env,
		Session:       sess,
		ExpiresAt:     time.Now().Add(s.ttl),
	}
	s.mu.Lock()
	s.items[t.Token] = t
	s.mu.Unlock()
	return t, nil
}

// Redeem 一次性兑换：命中即删除（含过期清理）；未命中/过期/已兑 → ErrTicketNotFound。
func (s *Store) Redeem(token string) (*Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.items[token]
	if !ok {
		return nil, ErrTicketNotFound
	}
	delete(s.items, token)
	if time.Now().After(t.ExpiresAt) {
		return nil, ErrTicketNotFound
	}
	return t, nil
}
