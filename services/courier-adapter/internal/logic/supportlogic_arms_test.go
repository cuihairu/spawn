package logic

// supportlogic.go 剩余补漏分支：validContextKey 两失败臂、mapping 的
// 未接入/未开通两臂、buildContext 的键形/值型/长度/总数越限与区服/订单
// 归属校验臂、catalogServer 的 fail-open 与区服不存在臂、serverIDOf 的
// float64/字符串/未知三臂、validateOrderOwnership 的未配置/请求失败/404/
// 其他状态/解码失败/归属不符臂、fetchCatalogServers 的请求/状态/解码失败臂、
// supportUpstreamError 的域码透传四臂，以及 List/Detail/Append/FAQ 的
// mapping/session/上游失败三态臂。
//
// 登记不可达（不测）：validateOrderOwnership/fetchCatalogServers 的
// io.ReadAll 失败臂（io.LimitReader 恒不报错）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/courier-adapter/courier"
	"github.com/tappi/tappi/services/courier-adapter/internal/types"
	"github.com/tappi/tappi/services/courier-adapter/model"
)

func TestValidContextKey_Arms(t *testing.T) {
	// 合法键（1-32 位 [a-zA-Z0-9_.]）
	if !validContextKey("a.b_1") {
		t.Fatal("valid key rejected")
	}
	// 空键 → 拒绝
	if validContextKey("") {
		t.Fatal("empty key accepted")
	}
	// 超长（>32）→ 拒绝
	if validContextKey(strings.Repeat("a", 33)) {
		t.Fatal("long key accepted")
	}
	// 非法字符 → 拒绝
	if validContextKey("bad!") {
		t.Fatal("bad char key accepted")
	}
}

func TestSupportMapping_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()
	sl := NewSupportLogic(ctx, e.base)

	// 未接入 → 404
	if _, err := sl.Create(&types.CreateSupportTicketRequest{Id: "g-miss"}, "", 77); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("missing mapping = %v, want 404", err)
	}

	// 已接入但未开通工单 → 403
	e.seedMapping("g-no", "cg", "prod", model.Capabilities{})
	if _, err := sl.Create(&types.CreateSupportTicketRequest{Id: "g-no"}, "", 77); !isAPIErr(err, http.StatusForbidden) {
		t.Fatalf("support disabled = %v, want 403", err)
	}

	// 映射查询失败（DROP 映射表）→ 500
	if _, err := dbExec(e, `DROP TABLE courier_game_mapping`); err != nil {
		t.Fatalf("drop mapping: %v", err)
	}
	if _, err := sl.Create(&types.CreateSupportTicketRequest{Id: "g1"}, "", 77); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("missing mapping table = %v, want 500", err)
	}
}

func TestSupportBuildContext_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()
	sl := NewSupportLogic(ctx, e.base)
	m := e.seedMapping("g1", "cg1", "prod", model.Capabilities{Support: true})

	// 非法键 → 400
	if _, err := sl.buildContext(m, "acc", &types.CreateSupportTicketRequest{Context: map[string]any{"bad!": "x"}}, 77); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("bad key = %v, want 400", err)
	}
	// 字符串值超 256 → 400
	if _, err := sl.buildContext(m, "acc", &types.CreateSupportTicketRequest{Context: map[string]any{"k": strings.Repeat("a", 257)}}, 77); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("long value = %v, want 400", err)
	}
	// bool / json.Number / float64 值型放行
	if _, err := sl.buildContext(m, "acc", &types.CreateSupportTicketRequest{Context: map[string]any{"b": true, "n": json.Number("5"), "f": 3.14}}, 77); err != nil {
		t.Fatalf("scalar values = %v, want nil", err)
	}
	// 其他值型（切片）→ 400
	if _, err := sl.buildContext(m, "acc", &types.CreateSupportTicketRequest{Context: map[string]any{"k": []any{}}}, 77); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("bad value type = %v, want 400", err)
	}
	// 键总数超 10（含自动附加 3）→ 400（值须为放行的标量型，避免先触发值型 400）
	big := map[string]any{}
	for i := 0; i < 11; i++ {
		big[strings.Repeat("k", 1)+string(rune('a'+i))] = float64(i)
	}
	if _, err := sl.buildContext(m, "acc", &types.CreateSupportTicketRequest{Context: big}, 77); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf(">10 keys = %v, want 400", err)
	}
	// orderId 超长 → 400
	if _, err := sl.buildContext(m, "acc", &types.CreateSupportTicketRequest{OrderId: strings.Repeat("a", 65)}, 77); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("long orderId = %v, want 400", err)
	}
	// 成功：自动附加事实覆盖保留键
	out, err := sl.buildContext(m, "acc", &types.CreateSupportTicketRequest{Context: map[string]any{"user": "x"}}, 77)
	if err != nil || out["spawn_user_id"] != int64(77) || out["game_id"] != "cg1" || out["courier_account_id"] != "acc" {
		t.Fatalf("auto facts = %+v/%v, want overwritten", out, err)
	}
}

func TestSupportCatalogServer_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()
	sl := NewSupportLogic(ctx, e.base)

	// 上游不可达 → fail-open（返回空名、无错）
	e.catalog.set(func(r *http.Request) (int, string) { return http.StatusInternalServerError, "x" })
	if name, err := sl.catalogServer("g1", "s1"); err != nil || name != "" {
		t.Fatalf("catalog fail-open = %q/%v, want empty/nil", name, err)
	}

	// 区服存在 → 返回名（float64 id 归一）
	e.catalog.set(func(r *http.Request) (int, string) {
		return http.StatusOK, `{"items":[{"id":123,"name":"srv-123"},{"id":"s2","name":"srv-2"}]}`
	})
	if name, err := sl.catalogServer("g1", "123"); err != nil || name != "srv-123" {
		t.Fatalf("float64 server = %q/%v, want srv-123", name, err)
	}
	if name, err := sl.catalogServer("g1", "s2"); err != nil || name != "srv-2" {
		t.Fatalf("string server = %q/%v, want srv-2", name, err)
	}

	// 区服不存在 → 400
	if _, err := sl.catalogServer("g1", "nope"); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("missing server = %v, want 400", err)
	}
}

func TestServerIDOf_Arms(t *testing.T) {
	if got := serverIDOf(map[string]any{"id": 123.0}); got != "123" {
		t.Fatalf("float id = %q, want 123", got)
	}
	if got := serverIDOf(map[string]any{"id": "s2"}); got != "s2" {
		t.Fatalf("string id = %q, want s2", got)
	}
	if got := serverIDOf(map[string]any{"id": true}); got != "" {
		t.Fatalf("unknown id = %q, want empty", got)
	}
}

func TestValidateOrderOwnership_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()
	sl := NewSupportLogic(ctx, e.base)
	m := e.seedMapping("g1", "cg1", "prod", model.Capabilities{Support: true})

	// token 未配置 → fail-open 放行
	e.base.Config.Payments.InternalToken = ""
	if err := sl.validateOrderOwnership(m, "o1", 77); err != nil {
		t.Fatalf("empty token = %v, want nil (fail-open)", err)
	}
	e.base.Config.Payments.InternalToken = "internal-tok"

	// base 未配置 → fail-open 放行
	e.base.Config.Payments.BaseURL = ""
	if err := sl.validateOrderOwnership(m, "o1", 77); err != nil {
		t.Fatalf("empty base = %v, want nil (fail-open)", err)
	}
	e.base.Config.Payments.BaseURL = e.payments.url()

	// NewRequest 失败（非法 base）→ fail-open
	e.base.Config.Payments.BaseURL = "://bad"
	if err := sl.validateOrderOwnership(m, "o1", 77); err != nil {
		t.Fatalf("bad base = %v, want nil (fail-open)", err)
	}
	e.base.Config.Payments.BaseURL = e.payments.url()

	// hc.Do 失败（关停端点）→ fail-open
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	e.base.Config.Payments.BaseURL = deadURL
	if err := sl.validateOrderOwnership(m, "o1", 77); err != nil {
		t.Fatalf("dead payments = %v, want nil (fail-open)", err)
	}
	e.base.Config.Payments.BaseURL = e.payments.url()

	// 404 → 400
	e.payments.set(func(r *http.Request) (int, string) { return http.StatusNotFound, `{}` })
	if err := sl.validateOrderOwnership(m, "o1", 77); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("404 = %v, want 400", err)
	}
	// 其他状态（500）→ fail-open
	e.payments.set(func(r *http.Request) (int, string) { return http.StatusInternalServerError, `{}` })
	if err := sl.validateOrderOwnership(m, "o1", 77); err != nil {
		t.Fatalf("500 = %v, want nil (fail-open)", err)
	}
	// 解码失败 → fail-open
	e.payments.set(func(r *http.Request) (int, string) { return http.StatusOK, "not-json" })
	if err := sl.validateOrderOwnership(m, "o1", 77); err != nil {
		t.Fatalf("non-json = %v, want nil (fail-open)", err)
	}
	// 归属不符（游戏或账号不匹配）→ 400
	e.payments.set(func(r *http.Request) (int, string) {
		return http.StatusOK, `{"order":{"game_id":"other","account_id":"77"}}`
	})
	if err := sl.validateOrderOwnership(m, "o1", 77); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("game mismatch = %v, want 400", err)
	}
	e.payments.set(func(r *http.Request) (int, string) {
		return http.StatusOK, `{"order":{"game_id":"g1","account_id":"99"}}`
	})
	if err := sl.validateOrderOwnership(m, "o1", 77); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("user mismatch = %v, want 400", err)
	}
	// 归属符合 → 放行
	e.payments.set(func(r *http.Request) (int, string) {
		return http.StatusOK, `{"order":{"game_id":"g1","account_id":"77"}}`
	})
	if err := sl.validateOrderOwnership(m, "o1", 77); err != nil {
		t.Fatalf("ownership ok = %v, want nil", err)
	}
}

func TestFetchCatalogServers_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()

	// NewRequest 失败
	e.base.Config.GameCatalog.BaseURL = "://bad"
	if _, err := fetchCatalogServers(ctx, e.base, "g1"); err == nil {
		t.Fatal("bad catalog url = nil, want error")
	}
	// hc.Do 失败
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	e.base.Config.GameCatalog.BaseURL = deadURL
	if _, err := fetchCatalogServers(ctx, e.base, "g1"); err == nil {
		t.Fatal("dead catalog = nil, want error")
	}
	// 非 200
	e2 := newArmsEnv(t)
	e2.catalog.set(func(r *http.Request) (int, string) { return http.StatusInternalServerError, "x" })
	if _, err := fetchCatalogServers(ctx, e2.base, "g1"); err == nil {
		t.Fatal("catalog 500 = nil, want error")
	}
	// 解码失败
	e3 := newArmsEnv(t)
	e3.catalog.set(func(r *http.Request) (int, string) { return http.StatusOK, "not-json" })
	if _, err := fetchCatalogServers(ctx, e3.base, "g1"); err == nil {
		t.Fatal("catalog non-json = nil, want error")
	}
	// 成功
	e4 := newArmsEnv(t)
	if items, err := fetchCatalogServers(ctx, e4.base, "g1"); err != nil || len(items) != 0 {
		t.Fatalf("catalog ok = %+v/%v, want empty items", items, err)
	}
}

func TestSupportUpstreamError_Arms(t *testing.T) {
	// 域码透传四臂
	if err := supportUpstreamError(courier.NewAPIError(404, "SUPPORT_TICKET_NOT_FOUND", "")); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("not found = %v, want 404", err)
	}
	if err := supportUpstreamError(courier.NewAPIError(409, "SUPPORT_TICKET_CLOSED", "")); !isAPIErr(err, http.StatusConflict) {
		t.Fatalf("closed = %v, want 409", err)
	}
	if err := supportUpstreamError(courier.NewAPIError(400, "COMMON_INVALID_ARGUMENT", "")); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("invalid arg = %v, want 400", err)
	}
	if err := supportUpstreamError(courier.NewAPIError(429, "RATE_LIMITED", "")); !isAPIErr(err, http.StatusTooManyRequests) {
		t.Fatalf("rate limited = %v, want 429", err)
	}
	// 其他域码 → 502
	if err := supportUpstreamError(courier.NewAPIError(500, "OTHER", "")); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("other code = %v, want 502", err)
	}
}

func TestSupportFlow_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()
	e.seedMapping("g1", "cg1", "prod", model.Capabilities{Support: true})
	sl := NewSupportLogic(ctx, e.base)

	// List/Detail/Append/FAQ：mapping 缺失 → 404（走 session 前即断）
	if _, err := sl.List(&types.SupportTicketListRequest{Id: "g-miss"}, "", 77); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("list missing mapping = %v, want 404", err)
	}
	if _, err := sl.Detail(&types.SupportTicketPathRequest{Id: "g-miss", TicketId: "tk1"}, "", 77); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("detail missing mapping = %v, want 404", err)
	}
	if _, err := sl.Append(&types.AppendSupportTicketRequest{Id: "g-miss", TicketId: "tk1", Body: "b"}, "", 77); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("append missing mapping = %v, want 404", err)
	}
	if _, err := sl.FAQ(&types.SupportFaqRequest{Id: "g-miss"}, "", 77); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("faq missing mapping = %v, want 404", err)
	}

	// session 失败（无绑定 + register 500）→ 502 传导
	e.courier.set(func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
	})
	if _, err := sl.List(&types.SupportTicketListRequest{Id: "g1"}, "", 99); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("list session fail = %v, want 502", err)
	}
	if _, err := sl.Detail(&types.SupportTicketPathRequest{Id: "g1", TicketId: "tk1"}, "", 99); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("detail session fail = %v, want 502", err)
	}
	if _, err := sl.Append(&types.AppendSupportTicketRequest{Id: "g1", TicketId: "tk1", Body: "b"}, "", 99); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("append session fail = %v, want 502", err)
	}
	if _, err := sl.FAQ(&types.SupportFaqRequest{Id: "g1"}, "", 99); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("faq session fail = %v, want 502", err)
	}

	// 上游失败（绑定成功但 Courier 工单调用 500）→ supportUpstreamError
	e.seedBinding(77, "acc_1")
	e.courier.set(func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
	})
	if _, err := sl.List(&types.SupportTicketListRequest{Id: "g1"}, "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("list upstream fail = %v, want 502", err)
	}
	if _, err := sl.Detail(&types.SupportTicketPathRequest{Id: "g1", TicketId: "tk1"}, "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("detail upstream fail = %v, want 502", err)
	}
	if _, err := sl.Append(&types.AppendSupportTicketRequest{Id: "g1", TicketId: "tk1", Body: "b"}, "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("append upstream fail = %v, want 502", err)
	}
	if _, err := sl.FAQ(&types.SupportFaqRequest{Id: "g1"}, "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("faq upstream fail = %v, want 502", err)
	}

	// Create：session 失败（无绑定 + register 500）→ 502
	e2 := newArmsEnv(t)
	e2.seedMapping("g1", "cg1", "prod", model.Capabilities{Support: true})
	e2.courier.set(func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
	})
	if _, err := NewSupportLogic(ctx, e2.base).Create(&types.CreateSupportTicketRequest{Id: "g1", Title: "t", Body: "b"}, "", 99); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("create session fail = %v, want 502", err)
	}
	// Create/List/Detail/FAQ：绑定成功但 Courier 工单调用 500 → supportUpstreamError
	e3 := newArmsEnv(t)
	e3.seedMapping("g1", "cg1", "prod", model.Capabilities{Support: true})
	e3.seedBinding(77, "acc_1")
	e3.courier.set(courierLoginOkSupport500)
	sl3 := NewSupportLogic(ctx, e3.base)
	if _, err := sl3.Create(&types.CreateSupportTicketRequest{Id: "g1", Title: "t", Body: "b"}, "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("create upstream fail = %v, want 502", err)
	}
	if _, err := sl3.List(&types.SupportTicketListRequest{Id: "g1"}, "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("list upstream fail = %v, want 502", err)
	}
	if _, err := sl3.Detail(&types.SupportTicketPathRequest{Id: "g1", TicketId: "tk1"}, "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("detail upstream fail = %v, want 502", err)
	}
	if _, err := sl3.FAQ(&types.SupportFaqRequest{Id: "g1"}, "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("faq upstream fail = %v, want 502", err)
	}

	// 健康存根：Create 成功（含审计）+ List/Detail/Append/FAQ 成功
	e.restoreCourier()
	if out, err := sl.Create(&types.CreateSupportTicketRequest{Id: "g1", Title: "t", Body: "b"}, "", 77); err != nil || out.Ticket.ID != "tk1" {
		t.Fatalf("create ok = %+v/%v, want tk1", out, err)
	}
	if _, err := sl.List(&types.SupportTicketListRequest{Id: "g1"}, "", 77); err != nil {
		t.Fatalf("list ok: %v", err)
	}
	if _, err := sl.Detail(&types.SupportTicketPathRequest{Id: "g1", TicketId: "tk1"}, "", 77); err != nil {
		t.Fatalf("detail ok: %v", err)
	}
	if _, err := sl.Append(&types.AppendSupportTicketRequest{Id: "g1", TicketId: "tk1", Body: "hi"}, "", 77); err != nil {
		t.Fatalf("append ok: %v", err)
	}
	if _, err := sl.FAQ(&types.SupportFaqRequest{Id: "g1", Keyword: "k", Limit: "5"}, "", 77); err != nil {
		t.Fatalf("faq ok: %v", err)
	}
}
