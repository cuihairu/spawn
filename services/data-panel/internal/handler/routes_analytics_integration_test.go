package handler

// GP-402 经营分析端到端（F24）：S2S 摄入（X-Internal-Token fail-closed、
// event_id 幂等、卫生校验）+ 名单管理员查询（漏斗/留存/活动效果/账单核对，
// 非名单 403、匿名 401）。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tappi/tappi/services/data-panel/internal/config"
	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/utils"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

const analyticsToken = "gp402-internal-token"

// newAnalyticsTestServer 真实 rest 服务器（含 S2S 摄入 + 名单查询路由），
// 摄入令牌 gp402-internal-token，管理员名单 user 7。
func newAnalyticsTestServer(t *testing.T, internalToken string) (string, func(int64) string) {
	t.Helper()
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var conf rest.RestConf
	conf.Port = port

	var c config.Config
	c.RestConf = conf
	c.Auth.JWTSecret = testSecret
	c.MySQL.DataSource = "file:" + t.TempDir() + "/stats.db"
	c.Internal.Token = internalToken
	c.Analytics.AdminUserIds = []string{"7"}

	svcCtx := svc.NewServiceContext(c)
	server := rest.MustNewServer(conf)
	RegisterHandlers(server, svcCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/api/v1/stats/users/1001/summary")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	sign := func(userId int64) string {
		now := time.Now()
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, utils.JWTClaims{
			UserId:   userId,
			Username: "tester",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(now),
			},
		}).SignedString([]byte(testSecret))
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}
		return token
	}
	return base, sign
}

// doIngest S2S 摄入（带可选 X-Internal-Token 头），返回状态码与原始 body。
func doIngest(t *testing.T, url, token, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build ingest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Internal-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	return resp.StatusCode, string(buf[:n])
}

func ingestEvents(events ...string) string {
	return `{"events":[` + strings.Join(events, ",") + `]}`
}

// doRaw 登录态请求，返回状态码与原始 body（analytics 断言用）。
func doRaw(t *testing.T, method, url, token, body string) (int, string) {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return resp.StatusCode, string(raw)
}

func gpEvent(id, typ string, userId int64, at string) string {
	return fmt.Sprintf(`{"event_id":%q,"event_type":%q,"user_id":%d,"occurred_at":%q}`, id, typ, userId, at)
}

// TestRoutes_AnalyticsIngestGuard 摄入鉴权 fail-closed + 卫生校验 + 幂等。
func TestRoutes_AnalyticsIngestGuard(t *testing.T) {
	base, _ := newAnalyticsTestServer(t, analyticsToken)
	url := base + "/internal/events"

	// 缺头 / 错令牌 → 401；正确令牌 → 200 accepted=2
	if code, _ := doIngest(t, url, "", ingestEvents(gpEvent("g1", "register", 1, "2026-11-01T09:00:00Z"))); code != http.StatusUnauthorized {
		t.Fatalf("missing token = %d, want 401", code)
	}
	if code, _ := doIngest(t, url, "wrong", ingestEvents(gpEvent("g1", "register", 1, "2026-11-01T09:00:00Z"))); code != http.StatusUnauthorized {
		t.Fatalf("wrong token = %d, want 401", code)
	}
	code, body := doIngest(t, url, analyticsToken, ingestEvents(
		gpEvent("g1", "register", 1, "2026-11-01T09:00:00Z"),
		gpEvent("g2", "login", 1, "2026-11-01T09:05:00Z"),
	))
	if code != http.StatusOK || !strings.Contains(body, `"accepted":2`) {
		t.Fatalf("ingest = %d body=%s, want accepted 2", code, body)
	}
	// 同批重放：event_id 幂等 → accepted 0
	code, body = doIngest(t, url, analyticsToken, ingestEvents(
		gpEvent("g1", "register", 1, "2026-11-01T09:00:00Z"),
		gpEvent("g2", "login", 1, "2026-11-01T09:05:00Z"),
	))
	if code != http.StatusOK || !strings.Contains(body, `"accepted":0`) {
		t.Fatalf("replay = %d body=%s, want accepted 0", code, body)
	}

	// 卫生校验：未知类型 / 空 event_id / 坏 occurred_at / 负金额 → 400
	if code, _ = doIngest(t, url, analyticsToken, ingestEvents(`{"event_id":"g3","event_type":"pageview","user_id":1}`)); code != http.StatusBadRequest {
		t.Fatalf("unknown type = %d, want 400", code)
	}
	if code, _ = doIngest(t, url, analyticsToken, ingestEvents(`{"event_id":"","event_type":"login","user_id":1}`)); code != http.StatusBadRequest {
		t.Fatalf("empty event_id = %d, want 400", code)
	}
	if code, _ = doIngest(t, url, analyticsToken, ingestEvents(`{"event_id":"g4","event_type":"login","user_id":1,"occurred_at":"not-a-time"}`)); code != http.StatusBadRequest {
		t.Fatalf("bad occurred_at = %d, want 400", code)
	}
	if code, _ = doIngest(t, url, analyticsToken, ingestEvents(`{"event_id":"g5","event_type":"order_paid","user_id":1,"amount_cents":-5}`)); code != http.StatusBadRequest {
		t.Fatalf("negative amount = %d, want 400", code)
	}
	if code, _ = doIngest(t, url, analyticsToken, `{"events":[]}`); code != http.StatusBadRequest {
		t.Fatalf("empty batch = %d, want 400", code)
	}
	// 批量上限 500：501 条 → 400
	many := make([]string, 501)
	for i := range many {
		many[i] = gpEvent("bulk-"+strconv.Itoa(i), "login", 1, "2026-11-01T10:00:00Z")
	}
	if code, _ = doIngest(t, url, analyticsToken, ingestEvents(many...)); code != http.StatusBadRequest {
		t.Fatalf("batch 501 = %d, want 400", code)
	}
}

// TestRoutes_AnalyticsQueries_FunnelRetentionRec 摄入后走四个查询端点：
// 匿名 401、非名单 403、名单管理员 200 且指标口径正确。
func TestRoutes_AnalyticsQueries_FunnelRetentionRec(t *testing.T) {
	base, sign := newAnalyticsTestServer(t, analyticsToken)
	admin := sign(7)
	other := sign(8)
	url := base + "/internal/events"

	// 事件集：
	//   11-01：u1 注册+登录、u2 注册+登录、u1 下载、u1 付费 600(game-a)
	//   11-02：u1 登录、u2 登录、u2 付费 900(game-b)
	//   11-03：u1 登录、u2 退款请求 900(game-b)
	//   11-01~03：活动 5 发奖：u1 x2(100+150)、u2 x1(200)
	batch := []string{
		gpEvent("q-r1", "register", 1, "2026-11-01T09:00:00Z"),
		gpEvent("q-l1", "login", 1, "2026-11-01T09:30:00Z"),
		gpEvent("q-r2", "register", 2, "2026-11-01T10:00:00Z"),
		gpEvent("q-l2", "login", 2, "2026-11-01T10:30:00Z"),
		gpEvent("q-d1", "download", 1, "2026-11-01T11:00:00Z"),
		`{"event_id":"q-p1","event_type":"order_paid","user_id":1,"game_id":"game-a","order_id":"ord-1","amount_cents":600,"occurred_at":"2026-11-01T12:00:00Z"}`,
		gpEvent("q-l3", "login", 1, "2026-11-02T09:30:00Z"),
		gpEvent("q-l4", "login", 2, "2026-11-02T10:30:00Z"),
		`{"event_id":"q-p2","event_type":"order_paid","user_id":2,"game_id":"game-b","order_id":"ord-2","amount_cents":900,"occurred_at":"2026-11-02T12:00:00Z"}`,
		gpEvent("q-l5", "login", 1, "2026-11-03T09:30:00Z"),
		`{"event_id":"q-f1","event_type":"refund_requested","user_id":2,"game_id":"game-b","order_id":"ord-2","amount_cents":900,"occurred_at":"2026-11-03T18:00:00Z"}`,
		`{"event_id":"q-w1","event_type":"reward_granted","user_id":1,"activity_id":5,"order_id":"ord-1","amount_cents":100,"occurred_at":"2026-11-01T12:05:00Z"}`,
		`{"event_id":"q-w2","event_type":"reward_granted","user_id":1,"activity_id":5,"order_id":"ord-2","amount_cents":150,"occurred_at":"2026-11-02T12:05:00Z"}`,
		`{"event_id":"q-w3","event_type":"reward_granted","user_id":2,"activity_id":5,"order_id":"ord-3","amount_cents":200,"occurred_at":"2026-11-03T12:05:00Z"}`,
	}
	if code, body := doIngest(t, url, analyticsToken, ingestEvents(batch...)); code != http.StatusOK || !strings.Contains(body, `"accepted":14`) {
		t.Fatalf("seed ingest = %d body=%s, want accepted 14", code, body)
	}

	q := "?from=2026-11-01&to=2026-11-30"

	// 匿名 401 / 非名单 403
	if status, _ := doRaw(t, http.MethodGet, base+"/api/v1/analytics/overview"+q, "", ""); status != http.StatusUnauthorized {
		t.Fatalf("anon overview = %d, want 401", status)
	}
	if status, _ := doRaw(t, http.MethodGet, base+"/api/v1/analytics/overview"+q, other, ""); status != http.StatusForbidden {
		t.Fatalf("non-admin overview = %d, want 403", status)
	}

	// 总览：按类型聚合（日期边界自动展开全天）
	status, body := doRaw(t, http.MethodGet, base+"/api/v1/analytics/overview"+q, admin, "")
	if status != http.StatusOK {
		t.Fatalf("overview = %d body=%v", status, body)
	}
	if !strings.Contains(body, `"event_type":"login"`) || !strings.Contains(body, `"events":5`) ||
		!strings.Contains(body, `"event_type":"order_paid"`) {
		t.Fatalf("overview body = %v", body)
	}

	// 类型过滤参数：未知类型 400
	if status, _ = doRaw(t, http.MethodGet, base+"/api/v1/analytics/overview?q=1&types=pageview", admin, ""); status != http.StatusBadRequest {
		t.Fatalf("bad types = %d, want 400", status)
	}

	// 漏斗：注册 2 → 活跃 2 → 下载 1 → 付费 2（u1/u2 各一笔）；
	// overall/step 按去重人数换算（付费人数可超下载人数：阶段口径非累计）
	status, body = doRaw(t, http.MethodGet, base+"/api/v1/analytics/funnel"+q, admin, "")
	if status != http.StatusOK {
		t.Fatalf("funnel = %d body=%v", status, body)
	}
	var funnel struct {
		Stages []struct {
			Stage       string  `json:"stage"`
			Users       int64   `json:"users"`
			Events      int64   `json:"events"`
			OverallConv float64 `json:"overall_conv"`
			StepConv    float64 `json:"step_conv"`
		} `json:"stages"`
	}
	if err := json.Unmarshal([]byte(body), &funnel); err != nil || len(funnel.Stages) != 4 {
		t.Fatalf("decode funnel: %v body=%v", err, body)
	}
	wantUsers := []int64{2, 2, 1, 2}
	wantStep := []float64{1, 1, 0.5, 2}
	for i, st := range funnel.Stages {
		if st.Users != wantUsers[i] || st.StepConv != wantStep[i] {
			t.Fatalf("stage %d = %+v, want users %d step %v", i, st, wantUsers[i], wantStep[i])
		}
	}
	if funnel.Stages[3].OverallConv != 1 {
		t.Fatalf("paid overall = %v, want 1", funnel.Stages[3].OverallConv)
	}

	// 留存：cohort = 窗口内首次活跃日。u1 首日 11-01（活跃 0/1/2 日），
	// u2 首日 11-01（活跃 0/1 日）→ 单一 cohort 11-01 size 2，
	// 第 0/1/2 日留存 {2,2,1} 人 = {1,1,0.5}
	status, body = doRaw(t, http.MethodGet, base+"/api/v1/analytics/retention?from=2026-11-01&to=2026-11-30&window=2", admin, "")
	if status != http.StatusOK {
		t.Fatalf("retention = %d body=%v", status, body)
	}
	var ret struct {
		EventType string `json:"event_type"`
		Cohorts   []struct {
			Day       string `json:"day"`
			Size      int64  `json:"size"`
			Retention map[string]struct {
				Retention float64 `json:"retention"`
				Users     int64   `json:"users"`
			} `json:"retention"`
		} `json:"cohorts"`
	}
	if err := json.Unmarshal([]byte(body), &ret); err != nil || ret.EventType != "login" || len(ret.Cohorts) != 1 {
		t.Fatalf("decode retention: %v body=%v", err, body)
	}
	if ret.Cohorts[0].Day != "2026-11-01" || ret.Cohorts[0].Size != 2 {
		t.Fatalf("cohort0 = %+v, want day 2026-11-01 size 2", ret.Cohorts[0])
	}
	c0 := ret.Cohorts[0].Retention
	if c0["0"].Users != 2 || c0["1"].Users != 2 || c0["2"].Users != 1 ||
		c0["0"].Retention != 1 || c0["2"].Retention != 0.5 {
		t.Fatalf("cohort0 retention = %+v, want day0 2/1 day1 2/1 day2 1/0.5", c0)
	}

	// 活动效果：activity 5 → 3 笔 / 2 人 / 450 分
	status, body = doRaw(t, http.MethodGet, base+"/api/v1/analytics/activities/5/effect", admin, "")
	if status != http.StatusOK || !strings.Contains(body, `"grants":3`) ||
		!strings.Contains(body, `"participants":2`) || !strings.Contains(body, `"reward_cents":450`) {
		t.Fatalf("activity effect = %d body=%v", status, body)
	}

	// 账单核对（F24）：game-a 付 1 笔 600 无退款；game-b 付 1 笔 900 退 1 笔 900
	status, body = doRaw(t, http.MethodGet, base+"/api/v1/analytics/reconciliation"+q, admin, "")
	if status != http.StatusOK {
		t.Fatalf("reconciliation = %d body=%v", status, body)
	}
	var rec struct {
		Items []struct {
			GameId      string `json:"game_id"`
			PaidCount   int64  `json:"paid_count"`
			PaidCents   int64  `json:"paid_cents"`
			RefundCount int64  `json:"refund_count"`
			RefundCents int64  `json:"refund_cents"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &rec); err != nil || len(rec.Items) != 2 {
		t.Fatalf("decode rec: %v body=%v", err, body)
	}
	got := map[string]string{}
	for _, it := range rec.Items {
		b, _ := json.Marshal(it)
		got[it.GameId] = string(b)
	}
	if s := got["game-a"]; !strings.Contains(s, `"paid_count":1`) || !strings.Contains(s, `"paid_cents":600`) ||
		strings.Contains(s, `"refund_count":1`) {
		t.Fatalf("game-a rec = %s", s)
	}
	if s := got["game-b"]; !strings.Contains(s, `"paid_cents":900`) || !strings.Contains(s, `"refund_count":1`) ||
		!strings.Contains(s, `"refund_cents":900`) {
		t.Fatalf("game-b rec = %s", s)
	}

	// 坏窗口：from > to → 400；坏日期 → 400
	if status, _ = doRaw(t, http.MethodGet, base+"/api/v1/analytics/overview?from=2026-11-30&to=2026-11-01", admin, ""); status != http.StatusBadRequest {
		t.Fatalf("from>to = %d, want 400", status)
	}
	if status, _ = doRaw(t, http.MethodGet, base+"/api/v1/analytics/overview?from=nope", admin, ""); status != http.StatusBadRequest {
		t.Fatalf("bad from = %d, want 400", status)
	}
}

// TestRoutes_AnalyticsIngestDisabled 空令牌配置 → 摄入整体拒绝 403（fail-closed）。
func TestRoutes_AnalyticsIngestDisabled(t *testing.T) {
	base, _ := newAnalyticsTestServer(t, "")
	code, _ := doIngest(t, base+"/internal/events", analyticsToken, ingestEvents(gpEvent("d1", "register", 1, "2026-11-01T09:00:00Z")))
	if code != http.StatusForbidden {
		t.Fatalf("disabled ingest = %d, want 403", code)
	}
}
