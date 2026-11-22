package logic

// 杂项补漏分支：EnsureSession 的绑定查询失败臂（DROP 绑定表）、autoBind 的
// 占位邮箱臂（user-service 回空邮箱）与审计失败仅记日志臂（DROP 审计表）、
// UpsertMapping 写成功但审计失败仅记日志臂（DROP 审计表），以及
// ErrorHandler 的非 apiError 兜底 400 臂。

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/courier-adapter/internal/types"
	"github.com/tappi/tappi/services/courier-adapter/model"
)

func TestEnsureSession_BindingQueryFailArm(t *testing.T) {
	e := newArmsEnv(t)
	e.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})
	if _, err := dbExec(e, `DROP TABLE platform_courier_binding`); err != nil {
		t.Fatalf("drop binding: %v", err)
	}
	// 映射 OK、绑定查询失败（非「查无」）→ 查询绑定失败 500
	if _, err := NewSessionLogic(context.Background(), e.base).GameSession("g1", "", 77); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("binding query fail = %v, want 500", err)
	}
}

func TestAutoBind_PlaceholderEmailAndAuditArms(t *testing.T) {
	// user-service 回空邮箱 → 落占位邮箱臂
	e := newArmsEnv(t)
	e.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})
	e.user.set(func(r *http.Request) (int, string) { return http.StatusOK, `{"code":200,"data":{"email":""}}` })
	if _, err := NewSessionLogic(context.Background(), e.base).GameSession("g1", "", 77); err != nil {
		t.Fatalf("placeholder email session = %v, want success", err)
	}

	// 审计表缺失 → 建号成功但审计失败仅记日志臂
	e2 := newArmsEnv(t)
	e2.seedMapping("g1", "cg1", "prod", model.Capabilities{Login: true})
	if _, err := dbExec(e2, `DROP TABLE adapter_audit_logs`); err != nil {
		t.Fatalf("drop audit: %v", err)
	}
	if _, err := NewSessionLogic(context.Background(), e2.base).GameSession("g1", "", 77); err != nil {
		t.Fatalf("autoBind audit fail = %v, want success (audit log-only)", err)
	}
}

func TestSupportAudit_FailArm(t *testing.T) {
	// 追加工单消息成功、审计失败仅记日志臂
	e := newArmsEnv(t)
	e.seedMapping("g1", "cg1", "prod", model.Capabilities{Support: true})
	e.seedBinding(77, "acc_1")
	if _, err := dbExec(e, `DROP TABLE adapter_audit_logs`); err != nil {
		t.Fatalf("drop audit: %v", err)
	}
	if _, err := NewSupportLogic(context.Background(), e.base).Append(
		&types.AppendSupportTicketRequest{Id: "g1", TicketId: "tk1", Body: "hi"}, "", 77); err != nil {
		t.Fatalf("append with audit fail = %v, want nil (audit log-only)", err)
	}
}

func TestUpsertMapping_AuditFailArm(t *testing.T) {
	// Upsert 写成功、审计失败仅记日志
	e := newArmsEnv(t)
	if _, err := dbExec(e, `DROP TABLE adapter_audit_logs`); err != nil {
		t.Fatalf("drop audit: %v", err)
	}
	if err := NewAdminLogic(context.Background(), e.base).UpsertMapping(
		&types.UpsertMappingRequest{Id: "g1", CourierGameId: "cg", PartnerId: 1}, "", 42); err != nil {
		t.Fatalf("upsert with audit fail = %v, want nil (audit log-only)", err)
	}
}

func TestErrorHandler_DefaultArms(t *testing.T) {
	// 非 apiError → 400 兜底 + 原消息
	code, msg := ErrorHandler(context.Background(), errors.New("plain failure"))
	if code != http.StatusBadRequest || msg != "plain failure" {
		t.Fatalf("ErrorHandler default = %d/%v, want 400/plain failure", code, msg)
	}
	// apiError → 原状态码 + 消息
	code, msg = ErrorHandler(context.Background(), newAPIError(http.StatusConflict, "dup"))
	if code != http.StatusConflict || msg != "dup" {
		t.Fatalf("ErrorHandler apiError = %d/%v, want 409/dup", code, msg)
	}
}
