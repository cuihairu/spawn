package logic

// sessionlogic.go 剩余补漏分支：GameSession 的映射缺失/能力关闭/EnsureSession
// 失败三臂与建号/代登录失败臂（courier 存根 500）、建号成功但落库/审计失败
// 臂（只读）、绑定查询失败臂（关闭库）、凭据解密失败臂（坏密文）、Redeem 的
// 票据不存在与会话缺失臂，courierUpstreamError 的域码透传/禁用/其他/非域
// 错误四臂，clientIP 兜底空值臂。
//
// 登记不可达（不测）：Tickets.Issue 失败臂与 GenerateAppPassword 失败臂
// （crypto/rand.Read 不可注入失败）；EncryptSecret 失败臂（密钥恒为合法
// 32 字节派生值）。

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/courier-adapter/courier"
	"github.com/tappi/tappi/services/courier-adapter/model"
)

func TestGameSession_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()

	// 映射缺失 → 404
	sl := NewSessionLogic(ctx, e.base)
	if _, err := sl.GameSession("g-miss", "", 77); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("missing mapping = %v, want 404", err)
	}

	// 能力未开通 → 403
	e.seedMapping("g-nologin", "cg", "prod", model.Capabilities{})
	if _, err := sl.GameSession("g-nologin", "", 77); !isAPIErr(err, http.StatusForbidden) {
		t.Fatalf("login disabled = %v, want 403", err)
	}

	// 无绑定 + 注册失败（courier 500）→ 域错误 502；邮箱获取失败落占位邮箱
	e.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})
	e.courier.set(func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
	})
	if _, err := sl.GameSession("g1", "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("register fail = %v, want 502", err)
	}

	// 已绑定 + 代登录失败（courier 仍 500）
	e.seedBinding(77, "acc_1")
	if _, err := sl.GameSession("g1", "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("login fail = %v, want 502", err)
	}

	// 绑定查询失败（关闭库）→ 500
	if _, err := NewSessionLogic(ctx, e.closedSvc()).GameSession("g1", "", 77); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("closed binding query = %v, want 500", err)
	}

	// 恢复健康存根：已绑定代登录成功 → 票据签发成功
	e.restoreCourier()
	resp, err := sl.GameSession("g1", "", 77)
	if err != nil || resp.Ticket == "" || resp.GameId != "cg1" {
		t.Fatalf("happy session = %+v/%v, want ticket", resp, err)
	}
	// 兑换成功 + 一次性
	if _, err := sl.Redeem(resp.Ticket); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	// EnsureSession 失败传导（注册失败 → GameSession 400/502）
	e.courier.set(func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
	})
	if _, err := sl.GameSession("g1", "", 99); err == nil {
		t.Fatal("ensure session fail = nil, want error")
	}
}

func TestGameSession_AutoBindFailArms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()
	e.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})

	// 无绑定 + 建号成功但落库失败（只读）→ 500；审计失败仅记日志
	ro := e.roSvc()
	if _, err := NewSessionLogic(ctx, ro).GameSession("g1", "", 88); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("readonly autobind = %v, want 500", err)
	}

	// 凭据解密失败（坏密文）→ 500
	e.restoreCourier()
	e.seedBinding(77, "acc_1")
	if _, err := dbExec(e, `UPDATE platform_courier_binding SET courier_secret_enc='!!!not-base64'`); err != nil {
		t.Fatalf("corrupt secret: %v", err)
	}
	if _, err := NewSessionLogic(ctx, e.base).GameSession("g1", "", 77); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("decrypt fail = %v, want 500", err)
	}
}

func TestRedeem_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()
	sl := NewSessionLogic(ctx, e.base)

	// 票据不存在 → 404
	if _, err := sl.Redeem("nope"); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("bad ticket = %v, want 404", err)
	}

	// 会话缺失（nil Session 票据）→ 500
	tk, err := e.base.Tickets.Issue(5, "cg", "prod", nil)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := sl.Redeem(tk.Token); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("nil session = %v, want 500", err)
	}
}

func TestCourierUpstreamErrorArms(t *testing.T) {
	// 域码透传：宽限期内 → 409
	if err := courierUpstreamError("p", courier.NewAPIError(409, "AUTH_ACCOUNT_PENDING_DELETE", "")); !isAPIErr(err, http.StatusConflict) {
		t.Fatalf("pending delete = %v, want 409", err)
	}
	// 域码透传：已禁用 → 403
	if err := courierUpstreamError("p", courier.NewAPIError(403, "AUTH_ACCOUNT_DISABLED", "")); !isAPIErr(err, http.StatusForbidden) {
		t.Fatalf("disabled = %v, want 403", err)
	}
	// 其他域码 → 502
	if err := courierUpstreamError("p", courier.NewAPIError(500, "OTHER", "")); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("other code = %v, want 502", err)
	}
	// 非 APIError → 502
	if err := courierUpstreamError("p", errors.New("plain")); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("plain error = %v, want 502", err)
	}
}

func TestClientIPFallbackArms(t *testing.T) {
	// ctx 无 client_ip → 兜底空串
	if got := clientIP(context.Background()); got != "" {
		t.Fatalf("clientIP fallback = %q, want empty", got)
	}
	// ctx 带 client_ip → 原样返回
	if got := clientIP(context.WithValue(context.Background(), "client_ip", "1.2.3.4")); got != "1.2.3.4" {
		t.Fatalf("clientIP value = %q, want 1.2.3.4", got)
	}
	// 值类型非 string → 兜底空串
	if got := clientIP(context.WithValue(context.Background(), "client_ip", 42)); got != "" {
		t.Fatalf("clientIP non-string = %q, want empty", got)
	}
}

// isAPIErr 断言 err 为 apiError 且状态码匹配。
func isAPIErr(err error, code int) bool {
	var ae *apiError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.StatusCode() == code && strings.Contains(err.Error(), ae.Error())
}
