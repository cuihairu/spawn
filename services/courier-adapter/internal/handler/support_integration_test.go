package handler

// GP-307 售后工单集成测试（F25）：提单（关联事实自动附加 + 区服归属校验 +
// 保留键不可覆盖）→ 列表/详情（处理记录 = 消息流）→ 追加（CLOSED 409 /
// 非本人 404）→ FAQ；能力未开通 403；审计留痕；scope 只走 header。

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/internal/config"
	"github.com/tappi/tappi/services/courier-adapter/internal/logic"
	"github.com/tappi/tappi/services/courier-adapter/internal/svc"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

type supportCourierStub struct {
	mu         sync.Mutex
	regN       int
	loginN     int
	createReqs []string // 提单 body 原文
	violations []string // game id 出现在 URL 的违规记录
}

func (s *supportCourierStub) violate(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.violations = append(s.violations, msg)
}

func (s *supportCourierStub) writeData(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"data":` + data + `}`))
}

func (s *supportCourierStub) writeErr(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"code":%q,"message":"stub","retryable":false}}`, code)
}

func (s *supportCourierStub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Courier-Game-Id") == "" || r.Header.Get("X-Courier-Env") == "" {
			s.violate("missing scope header")
		}
		// scope 传输约束：game id 只许 header，URL 不得携带（context 体内
		// 自动附加 gameId 是契约 SDK 口径，不算传输违规）
		gameID := r.Header.Get("X-Courier-Game-Id")
		if gameID != "" && (strings.Contains(r.URL.Path, gameID) || strings.Contains(r.URL.RawQuery, gameID)) {
			s.violate("game id leaked into URL: " + r.URL.String())
		}

		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/identity/register"):
			s.mu.Lock()
			s.regN++
			s.mu.Unlock()
			s.writeData(w, `{"account":{"id":"acc_reg_1","type":"EMAIL","status":"ACTIVE"},"accessToken":"atk-1","accessExpiresAt":"2026-11-09T00:15:00.000Z","refreshToken":"rtk","refreshExpiresAt":"2026-12-09T00:00:00.000Z"}`)
		case strings.HasSuffix(path, "/identity/login"):
			s.mu.Lock()
			s.loginN++
			s.mu.Unlock()
			s.writeData(w, `{"account":{"id":"acc_reg_1","type":"EMAIL","status":"ACTIVE"},"accessToken":"atk-1","accessExpiresAt":"2026-11-09T00:15:00.000Z","refreshToken":"rtk","refreshExpiresAt":"2026-12-09T00:00:00.000Z"}`)
		case strings.HasSuffix(path, "/support/tickets") && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			s.mu.Lock()
			s.createReqs = append(s.createReqs, string(raw))
			s.mu.Unlock()
			var req struct {
				Title   string         `json:"title"`
				Context map[string]any `json:"context"`
			}
			_ = json.Unmarshal(raw, &req)
			echo, _ := json.Marshal(req.Context)
			s.writeData(w, fmt.Sprintf(`{"id":"tkt_1","title":%q,"status":"OPEN","category":"PAYMENT","context":%s,"createdAt":"2026-11-09T00:00:00.000Z","updatedAt":"2026-11-09T00:00:00.000Z"}`, req.Title, echo))
		case strings.HasSuffix(path, "/support/tickets") && r.Method == http.MethodGet:
			s.writeData(w, `{"items":[{"id":"tkt_1","title":"订单退款","status":"REPLIED","category":"PAYMENT","createdAt":"2026-11-09T00:00:00.000Z","updatedAt":"2026-11-09T01:00:00.000Z"}],"nextCursor":""}`)
		case strings.HasPrefix(path, "/v1/support/tickets/") && strings.HasSuffix(path, "/messages") && r.Method == http.MethodPost:
			ticketID := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/support/tickets/"), "/messages")
			if ticketID == "tkt_closed" {
				s.writeErr(w, http.StatusConflict, "SUPPORT_TICKET_CLOSED")
				return
			}
			if ticketID == "tkt_missing" {
				s.writeErr(w, http.StatusNotFound, "SUPPORT_TICKET_NOT_FOUND")
				return
			}
			s.writeData(w, `{"senderType":"PLAYER","body":"补充截图已传","createdAt":"2026-11-09T02:00:00.000Z"}`)
		case strings.HasPrefix(path, "/v1/support/tickets/") && r.Method == http.MethodGet:
			ticketID := strings.TrimPrefix(path, "/v1/support/tickets/")
			if ticketID == "tkt_missing" {
				s.writeErr(w, http.StatusNotFound, "SUPPORT_TICKET_NOT_FOUND")
				return
			}
			// 处理记录：PLAYER 首报 + AGENT 回复（升序）
			s.writeData(w, `{"ticket":{"id":"tkt_1","title":"订单退款","status":"REPLIED","category":"PAYMENT","context":{"order_id":"ord-9"},"createdAt":"2026-11-09T00:00:00.000Z","updatedAt":"2026-11-09T01:00:00.000Z"},"messages":[{"senderType":"PLAYER","body":"订单未到账","createdAt":"2026-11-09T00:00:00.000Z"},{"senderType":"AGENT","body":"已补发，请查收","createdAt":"2026-11-09T01:00:00.000Z"}]}`)
		case strings.HasSuffix(path, "/support/faq"):
			s.writeData(w, `{"items":[{"id":"faq_1","question":"充值未到账怎么办","answer":"先查订单状态","keywords":["充值","到账"]}],"nextCursor":""}`)
		default:
			s.writeErr(w, http.StatusNotFound, "COMMON_NOT_FOUND")
		}
	}
}

func (s *supportCourierStub) createBodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.createReqs...)
}

// supportCatalogStub /me 角色权威（42=operator）+ 公开区服表（g1：5=S1 open）。
func supportCatalogStub() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/me" {
			role := "player"
			if roleForAuthHeader(r.Header.Get("Authorization")) == "operator" {
				role = "operator"
			}
			_, _ = fmt.Fprintf(w, `{"userId":1,"username":"u","role":%q,"partnerId":0}`, role)
			return
		}
		if r.URL.Path == "/games/g1/servers" {
			_, _ = w.Write([]byte(`{"items":[{"id":5,"game_id":"g1","name":"S1","region":"华东","status":"open","sort":1,"planned_open_at":""},{"id":6,"game_id":"g1","name":"S2","region":"华北","status":"closed","sort":2,"planned_open_at":""}]}`))
			return
		}
		http.NotFound(w, r)
	}
}

func newSupportEnv(t *testing.T) (string, *supportCourierStub) {
	t.Helper()
	stub := &supportCourierStub{}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var cfg config.Config
	cfg.Host = "127.0.0.1"
	cfg.Port = port
	cfg.RestConf.Timeout = 5000
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.MySQL.DataSource = "file:" + t.TempDir() + "/adapter-support.db"
	cfg.Courier.BaseURL = httptest.NewServer(stub.handler()).URL
	cfg.Courier.Timeout = 2000
	cfg.Credential.Key = "adapter-test-credential-key"
	cfg.GameCatalog.BaseURL = httptest.NewServer(supportCatalogStub()).URL
	cfg.GameCatalog.Timeout = 2000
	cfg.UserService.BaseURL = httptest.NewServer(userStub()).URL
	cfg.UserService.Timeout = 2000
	cfg.Ticket.TTLSeconds = 120

	svcCtx := svc.NewServiceContext(cfg)
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)
	server := rest.MustNewServer(cfg.RestConf)
	RegisterHandlers(server, svcCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test server did not become ready in time")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return base, stub
}

func TestRoutes_SupportTickets(t *testing.T) {
	base, stub := newSupportEnv(t)
	player := signToken(t, 77, "player")
	operator := signToken(t, 42, "ops")

	// 1. 映射：g1 开 support；g2 未开（403 前置）；g3 未接入（404）
	if code, body := doJSON(t, http.MethodPut, base+"/admin/games/g1/mapping", operator,
		`{"courierGameId":"cg1","env":"prod","partnerId":1,"capabilities":{"login":true,"support":true}}`); code != http.StatusOK {
		t.Fatalf("upsert g1 = %d body=%s", code, body)
	}
	if code, body := doJSON(t, http.MethodPut, base+"/admin/games/g2/mapping", operator,
		`{"courierGameId":"cg2","env":"prod","partnerId":1,"capabilities":{"login":true}}`); code != http.StatusOK {
		t.Fatalf("upsert g2 = %d body=%s", code, body)
	}

	// 2. 未登录 401；未接入 404；能力未开 403
	if code, _ := doJSON(t, http.MethodPost, base+"/games/g1/support/tickets", "",
		`{"title":"t","body":"b"}`); code != http.StatusUnauthorized {
		t.Fatalf("anon create = %d, want 401", code)
	}
	if code, _ := doJSON(t, http.MethodPost, base+"/games/g3/support/tickets", player,
		`{"title":"t","body":"b"}`); code != http.StatusNotFound {
		t.Fatalf("unmapped create = %d, want 404", code)
	}
	if code, _ := doJSON(t, http.MethodPost, base+"/games/g2/support/tickets", player,
		`{"title":"t","body":"b"}`); code != http.StatusForbidden {
		t.Fatalf("support-disabled create = %d, want 403", code)
	}

	// 3. 提单：区服归属校验 + 订单透传 + 保留键不可覆盖（spawn_user_id/game_id）
	code, body := doJSON(t, http.MethodPost, base+"/games/g1/support/tickets", player,
		`{"title":"订单未到账","body":"充值后未收到","category":"PAYMENT","serverId":"5","orderId":"ord-9","context":{"level":42,"spawn_user_id":1,"game_id":"fake"}}`)
	if code != http.StatusOK {
		t.Fatalf("create ticket = %d body=%s", code, body)
	}
	var created struct {
		Ticket struct {
			ID      string         `json:"id"`
			Status  string         `json:"status"`
			Context map[string]any `json:"context"`
		} `json:"ticket"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.Ticket.ID != "tkt_1" || created.Ticket.Status != "OPEN" {
		t.Fatalf("decode create = %v body=%s", err, body)
	}
	bodies := stub.createBodies()
	if len(bodies) != 1 {
		t.Fatalf("courier create calls = %d, want 1", len(bodies))
	}
	var sent struct {
		Context map[string]any `json:"context"`
	}
	if err := json.Unmarshal([]byte(bodies[0]), &sent); err != nil {
		t.Fatalf("decode sent: %v", err)
	}
	want := map[string]any{
		"spawn_user_id":      float64(77), // 自动附加，业务不可覆盖
		"courier_account_id": "acc_reg_1",
		"game_id":            "cg1",
		"server_id":          "5", // 区服校验通过后入 context
		"server_name":        "S1",
		"order_id":           "ord-9",
		"level":              float64(42),
	}
	for k, v := range want {
		got, ok := sent.Context[k]
		if !ok || got != v {
			t.Fatalf("context[%s] = %v (%v), want %v; full=%v", k, got, ok, v, sent.Context)
		}
	}

	// 4. 区服不属于该游戏 → 400
	if code, _ = doJSON(t, http.MethodPost, base+"/games/g1/support/tickets", player,
		`{"title":"t","body":"b","serverId":"99"}`); code != http.StatusBadRequest {
		t.Fatalf("bad server create = %d, want 400", code)
	}

	// 5. 列表（我的工单，updatedAt 倒序透传）
	code, body = doJSON(t, http.MethodGet, base+"/games/g1/support/tickets?limit=20", player, "")
	if code != http.StatusOK || !strings.Contains(body, `"id":"tkt_1"`) || !strings.Contains(body, `"status":"REPLIED"`) {
		t.Fatalf("list = %d body=%s", code, body)
	}

	// 6. 详情：处理记录 = 消息流（PLAYER + AGENT 升序）
	code, body = doJSON(t, http.MethodGet, base+"/games/g1/support/tickets/tkt_1", player, "")
	if code != http.StatusOK {
		t.Fatalf("detail = %d body=%s", code, body)
	}
	var detail struct {
		Ticket struct {
			ID string `json:"id"`
		} `json:"ticket"`
		Messages []struct {
			SenderType string `json:"senderType"`
			Body       string `json:"body"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &detail); err != nil || detail.Ticket.ID != "tkt_1" ||
		len(detail.Messages) != 2 || detail.Messages[0].SenderType != "PLAYER" || detail.Messages[1].SenderType != "AGENT" {
		t.Fatalf("detail decode = %v body=%s", err, body)
	}

	// 7. 追加消息；CLOSED 409；非本人/不存在 404
	code, body = doJSON(t, http.MethodPost, base+"/games/g1/support/tickets/tkt_1/messages", player,
		`{"body":"补充截图已传"}`)
	if code != http.StatusOK || !strings.Contains(body, `"senderType":"PLAYER"`) {
		t.Fatalf("append = %d body=%s", code, body)
	}
	if code, _ = doJSON(t, http.MethodPost, base+"/games/g1/support/tickets/tkt_closed/messages", player,
		`{"body":"x"}`); code != http.StatusConflict {
		t.Fatalf("append closed = %d, want 409", code)
	}
	if code, _ = doJSON(t, http.MethodPost, base+"/games/g1/support/tickets/tkt_missing/messages", player,
		`{"body":"x"}`); code != http.StatusNotFound {
		t.Fatalf("append missing = %d, want 404", code)
	}

	// 8. FAQ 检索
	code, body = doJSON(t, http.MethodGet, base+"/games/g1/support/faq?keyword=%E5%85%85%E5%80%BC", player, "")
	if code != http.StatusOK || !strings.Contains(body, `"id":"faq_1"`) {
		t.Fatalf("faq = %d body=%s", code, body)
	}

	// 9. 审计留痕（support_ticket_create + support_ticket_append）
	code, body = doJSON(t, http.MethodGet, base+"/admin/audit", operator, "")
	if code != http.StatusOK || !strings.Contains(body, "support_ticket_create") || !strings.Contains(body, "support_ticket_append") {
		t.Fatalf("audit = %d body=%s", code, body)
	}

	// 10. scope 只走 header：无 URL 泄漏违规
	if len(stub.violations) > 0 {
		t.Fatalf("scope violations: %v", stub.violations)
	}
}
