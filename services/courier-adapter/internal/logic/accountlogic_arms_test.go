package logic

// accountlogic.go 剩余补漏分支：Delete 的 mappingAndBinding/proxyLogin/
// Courier.DeleteAccount 三失败臂与 DELETED/默认状态 UpdateStatus 记日志
// 臂（只读）与审计失败臂（只读）；CancelDelete 的空 token/上游失败/只读三臂；
// mappingAndBinding 的映射缺失/绑定缺失（404）与查询失败（500，含 DROP
// 绑定表使绑定查询单独失败）；proxyLogin 的解密失败（坏密文）与代登录失败
// （courier 500）臂。

import (
	"context"
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/courier-adapter/model"
)

func TestAccountDelete_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()
	al := NewAccountLogic(ctx, e.base)

	// 映射缺失 → 404（mappingAndBinding 首臂）
	if _, err := al.Delete("g-miss", "", 77); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("missing mapping = %v, want 404", err)
	}

	// 绑定缺失 → 404
	e.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})
	if _, err := al.Delete("g1", "", 77); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("missing binding = %v, want 404", err)
	}

	// 映射查询失败（关闭库）→ 500
	if _, err := NewAccountLogic(ctx, e.closedSvc()).Delete("g1", "", 77); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("closed mapping query = %v, want 500", err)
	}

	// 映射 OK + 绑定表缺失 → 绑定查询失败 500（mappingAndBinding 第四臂）
	e2 := newArmsEnv(t)
	e2.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})
	if _, err := dbExec(e2, `DROP TABLE platform_courier_binding`); err != nil {
		t.Fatalf("drop binding table: %v", err)
	}
	if _, err := NewAccountLogic(ctx, e2.base).Delete("g1", "", 77); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("missing binding table = %v, want 500", err)
	}

	// 凭据解密失败（坏密文）→ proxyLogin 500
	e3 := newArmsEnv(t)
	e3.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})
	e3.seedBinding(77, "acc_1")
	if _, err := dbExec(e3, `UPDATE platform_courier_binding SET courier_secret_enc='!!!not-base64'`); err != nil {
		t.Fatalf("corrupt secret: %v", err)
	}
	if _, err := NewAccountLogic(ctx, e3.base).Delete("g1", "", 77); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("decrypt fail = %v, want 500", err)
	}

	// 代登录失败（courier 500）→ 502
	e4 := newArmsEnv(t)
	e4.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})
	e4.seedBinding(77, "acc_1")
	e4.courier.set(func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
	})
	if _, err := NewAccountLogic(ctx, e4.base).Delete("g1", "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("proxy login fail = %v, want 502", err)
	}

	// Courier.DeleteAccount 失败 → 502（含记日志臂）
	e5 := newArmsEnv(t)
	e5.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})
	e5.seedBinding(77, "acc_1")
	e5.courier.set(func(r *http.Request) (int, string) {
		if containsPath(r, "account/delete") {
			return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
		}
		return defaultCourier(r)
	})
	if _, err := NewAccountLogic(ctx, e5.base).Delete("g1", "", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("delete upstream fail = %v, want 502", err)
	}
}

func TestAccountDelete_WriteFailArms(t *testing.T) {
	// 只读库：读成功、代登录成功、注销受理成功，UpdateStatus/审计失败仅记日志
	// → 默认（PENDING_DELETE）分支 52 + 审计 57；DELETED 分支 47/48。
	for _, status := range []struct {
		name   string
		body   string
		branch string
	}{
		{"pending_default", `{"data":{"status":"PENDING_DELETE","cancelToken":"ct-1","pendingDelete":true}}`, "default"},
		{"deleted", `{"data":{"status":"DELETED","deletedAt":"d"}}`, "deleted"},
	} {
		t.Run(status.name, func(t *testing.T) {
			e := newArmsEnv(t)
			e.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})
			e.seedBinding(77, "acc_1")
			e.courier.set(func(r *http.Request) (int, string) {
				if containsPath(r, "account/delete") && !containsPath(r, "cancel") {
					return http.StatusOK, status.body
				}
				return defaultCourier(r)
			})
			out, err := NewAccountLogic(context.Background(), e.roSvc()).Delete("g1", "", 77)
			if err != nil || out.Status == "" {
				t.Fatalf("readonly delete = %+v/%v, want success with write-fail logs", out, err)
			}
		})
	}
}

func TestAccountCancelDelete_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()

	// 空 token → 400
	if err := NewAccountLogic(ctx, e.base).CancelDelete("g1", "", 77); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("empty token = %v, want 400", err)
	}

	// 映射缺失 → 404
	if err := NewAccountLogic(ctx, e.base).CancelDelete("g-miss", "ct", 77); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("missing mapping = %v, want 404", err)
	}

	// 上游撤销失败 → 502
	e.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})
	e.seedBinding(77, "acc_1")
	e.courier.set(func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
	})
	if err := NewAccountLogic(ctx, e.base).CancelDelete("g1", "ct", 77); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("cancel upstream fail = %v, want 502", err)
	}

	// 只读：撤销成功但 UpdateStatus/审计失败仅记日志
	e.restoreCourier()
	if err := NewAccountLogic(ctx, e.roSvc()).CancelDelete("g1", "ct", 77); err != nil {
		t.Fatalf("readonly cancel = %v, want success with write-fail logs", err)
	}
}
