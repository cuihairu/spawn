package handler

// GP-306/401 业务通知投递（server_open 开服提醒）本地 outbox 补偿集成测试：
// community 短时不可达（503）→ 开服主流程不受影响（仍 200），通知先落
// outbox（pending）；下游恢复后 Relay 补投成功标记 delivered，扇出完整到达。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

// notifyDownStub community /internal/notifications 桩：记录每次投递，可切换
// 下游不可达（503）。令牌校验与既有 communityStub 同款（gp306Token）。
type notifyDownStub struct {
	mu     sync.Mutex
	reqs   [][]byte
	down   bool
	srv    *httptest.Server
	create func(userIDs []int64) int
}

func newNotifyDownStub() *notifyDownStub {
	s := &notifyDownStub{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/notifications", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.reqs = append(s.reqs, raw)
		down := s.down
		s.mu.Unlock()
		if r.Header.Get("X-Internal-Token") != gp306Token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if down {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var body struct {
			UserIds []int64 `json:"user_ids"`
		}
		_ = json.Unmarshal(raw, &body)
		n, _ := json.Marshal(len(body.UserIds))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"created":` + string(n) + `}`))
	})
	s.srv = httptest.NewServer(mux)
	return s
}

func (s *notifyDownStub) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.reqs)
}

func (s *notifyDownStub) Last() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reqs) == 0 {
		return nil
	}
	return s.reqs[len(s.reqs)-1]
}

func (s *notifyDownStub) setDown(d bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.down = d
}

// TestRoutes_NotifyOutboxRetry 开服提醒投递补偿闭环：下游 503 → 开服 200 且落
// pending；Relay 首投失败退避；下游恢复 + 退避拨回 → Relay 补投成功，扇出带
// type=server_open 与预约用户，队列清空。
func TestRoutes_NotifyOutboxRetry(t *testing.T) {
	stub := newNotifyDownStub()
	defer stub.srv.Close()
	base, sign, svcCtx := newReservationTestServer(t, stub.srv.URL)
	admin := sign(claimsFor(1, "admin"))
	player := sign(claimsFor(7, "bob"))
	player2 := sign(claimsFor(8, "carol"))
	code := 0
	body := ""

	// 造合作方 + 已上架游戏 + 预约区服（GP-306 同款前置）
	gameBody := `{"title":"通知补偿GP306","description":"notify-outbox","genres":["MMO"],"platforms":["Android"],"release_date":"2026-11-09","developer":"d","publisher":"p","tags":[],"score":8,"cover_image":"https://example.com/cover.jpg"}`
	code, body = doAuthed(t, http.MethodPost, base+"/games", admin, gameBody)
	if code != http.StatusOK {
		t.Fatalf("create game = %d body=%s", code, body)
	}
	var gameResp struct {
		Game struct {
			Id string `json:"id"`
		} `json:"game"`
	}
	_ = json.Unmarshal([]byte(body), &gameResp)
	gameID := gameResp.Game.Id
	var partnerResp struct {
		Id int64 `json:"id"`
	}
	_, body = doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"通知厂GP306"}`)
	_ = json.Unmarshal([]byte(body), &partnerResp)
	partnerUserID := partnerResp.Id + 100
	partnerIDStr := strconv.FormatInt(partnerResp.Id, 10)
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/roles", admin,
		`{"user_id":`+strconv.FormatInt(partnerUserID, 10)+`,"role":"partner","partner_id":`+partnerIDStr+`}`); code != http.StatusOK {
		t.Fatalf("grant partner = %d", code)
	}
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/games/"+gameID+"/ownership", admin,
		`{"partner_id":`+partnerIDStr+`}`); code != http.StatusOK {
		t.Fatalf("assign ownership = %d", code)
	}
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/games/"+gameID+"/publish", admin, ""); code != http.StatusOK {
		t.Fatalf("publish game = %d", code)
	}
	partner := sign(claimsFor(partnerUserID, "partner-gp306"))
	code, body = doAuthed(t, http.MethodPost, base+"/partner/games/"+gameID+"/servers", partner,
		`{"name":"S1","region":"华东","status":"closed"}`)
	if code != http.StatusOK {
		t.Fatalf("create server = %d body=%s", code, body)
	}
	var serverResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &serverResp)
	serverID := serverResp.Id
	serverIDStr := strconv.FormatInt(serverID, 10)
	if code, _ = doAuthed(t, http.MethodPost, base+"/games/"+gameID+"/servers/"+serverIDStr+"/reserve", player, ""); code != http.StatusOK {
		t.Fatalf("player reserve = %d", code)
	}
	if code, _ = doAuthed(t, http.MethodPost, base+"/games/"+gameID+"/servers/"+serverIDStr+"/reserve", player2, ""); code != http.StatusOK {
		t.Fatalf("player2 reserve = %d", code)
	}

	// 下游不可达：开服仍 200（通知投递不影响主流程），通知落 outbox pending
	stub.setDown(true)
	if code, _ = doAuthed(t, http.MethodPatch, base+"/partner/games/"+gameID+"/servers/"+serverIDStr, partner, `{"status":"open"}`); code != http.StatusOK {
		t.Fatalf("open server down = %d", code)
	}
	if stub.Calls() != 1 {
		t.Fatalf("down: notify attempts = %d, want 1", stub.Calls())
	}
	rows, err := svcCtx.NotifyOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("outbox pending = %d err=%v, want 1", len(rows), err)
	}
	row := rows[0]
	if row.NotifyType != "server_open" || row.Status != "pending" || row.Attempts != 0 {
		t.Fatalf("outbox row = %+v, want server_open/pending/0 attempts", row)
	}

	// 首轮 Relay：仍失败 → attempts=1 + 退避延期
	ctx := context.Background()
	attempted, delivered := svcCtx.NotifyRelay.RelayOnce(ctx)
	if attempted != 1 || delivered != 0 {
		t.Fatalf("relay down = %d/%d, want 1/0", attempted, delivered)
	}
	rows, _ = svcCtx.NotifyOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 1 || rows[0].Attempts != 1 {
		t.Fatalf("outbox after failed relay = %+v, want attempts=1", rows)
	}

	// 下游恢复 + 退避到期（把 next_retry_at 拨回过去模拟时间推进）
	stub.setDown(false)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := svcCtx.NotifyOutbox.MarkFailed(row.NotifyKey, 1, "模拟退避到期", past); err != nil {
		t.Fatalf("rewind backoff: %v", err)
	}
	attempted, delivered = svcCtx.NotifyRelay.RelayOnce(ctx)
	if attempted != 1 || delivered != 1 {
		t.Fatalf("relay recovered = %d/%d, want 1/1", attempted, delivered)
	}
	raw := stub.Last()
	var fanout struct {
		UserIds []int64 `json:"user_ids"`
		Type    string  `json:"type"`
		Target  int64   `json:"target_id"`
	}
	if err := json.Unmarshal(raw, &fanout); err != nil ||
		fanout.Type != "server_open" || fanout.Target != serverID ||
		len(fanout.UserIds) != 2 || fanout.UserIds[0] != 7 || fanout.UserIds[1] != 8 {
		t.Fatalf("relayed fanout = %s err=%v", raw, err)
	}
	rows, _ = svcCtx.NotifyOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("outbox pending after delivered = %d, want 0", len(rows))
	}
}
