package logic

// mappinglogic.go 剩余补漏分支：UpsertMapping 的空 courierGameId（400）、
// env 默认 prod、非法 env（400）与写失败（只读 500）/审计失败臂；
// GetMapping 的 requireAdminRole 失败传导、映射缺失 404 与查询失败 500
// （关闭库）；ListAudit 的非管理员 403 与查询失败 500（关闭库）。

import (
	"context"
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/courier-adapter/internal/types"
)

func TestUpsertMapping_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()
	al := NewAdminLogic(ctx, e.base)

	// 空 courierGameId → 400
	if err := al.UpsertMapping(&types.UpsertMappingRequest{Id: "g1", CourierGameId: "  "}, "", 42); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("empty courierGameId = %v, want 400", err)
	}

	// env 默认 prod
	if err := al.UpsertMapping(&types.UpsertMappingRequest{Id: "g-def", CourierGameId: "cg", Env: "", PartnerId: 1}, "", 42); err != nil {
		t.Fatalf("default env upsert: %v", err)
	}
	if m, err := e.base.Mappings.Get("g-def"); err != nil || m.Env != "prod" {
		t.Fatalf("default env mapping = %+v/%v, want env prod", m, err)
	}

	// 非法 env → 400
	if err := al.UpsertMapping(&types.UpsertMappingRequest{Id: "g1", CourierGameId: "cg", Env: "BAD!"}, "", 42); !isAPIErr(err, http.StatusBadRequest) {
		t.Fatalf("bad env = %v, want 400", err)
	}

	// 写失败（只读）→ 500；审计失败仅记日志
	if err := NewAdminLogic(ctx, e.roSvc()).UpsertMapping(&types.UpsertMappingRequest{Id: "g1", CourierGameId: "cg"}, "", 42); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("readonly upsert = %v, want 500", err)
	}
}

func TestGetMapping_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()

	// 非管理员角色 → requireAdminRole 403 传导
	e.catalog.set(func(r *http.Request) (int, string) { return http.StatusOK, `{"role":"player","partnerId":0}` })
	if _, err := NewAdminLogic(ctx, e.base).GetMapping("g1", ""); !isAPIErr(err, http.StatusForbidden) {
		t.Fatalf("player get mapping = %v, want 403", err)
	}

	// 管理员 + 映射缺失 → 404
	e.catalog.set(func(r *http.Request) (int, string) { return http.StatusOK, `{"role":"operator","partnerId":1}` })
	if _, err := NewAdminLogic(ctx, e.base).GetMapping("g-miss", ""); !isAPIErr(err, http.StatusNotFound) {
		t.Fatalf("missing mapping = %v, want 404", err)
	}

	// 管理员 + 查询失败（关闭库）→ 500
	if _, err := NewAdminLogic(ctx, e.closedSvc()).GetMapping("g1", ""); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("closed mapping query = %v, want 500", err)
	}
}

func TestListAudit_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()

	// 非管理员 → 403
	e.catalog.set(func(r *http.Request) (int, string) { return http.StatusOK, `{"role":"player","partnerId":0}` })
	if _, err := NewAdminLogic(ctx, e.base).ListAudit(""); !isAPIErr(err, http.StatusForbidden) {
		t.Fatalf("player list audit = %v, want 403", err)
	}

	// 管理员 + 查询失败（关闭库）→ 500
	e.catalog.set(func(r *http.Request) (int, string) { return http.StatusOK, `{"role":"operator","partnerId":1}` })
	if _, err := NewAdminLogic(ctx, e.closedSvc()).ListAudit(""); !isAPIErr(err, http.StatusInternalServerError) {
		t.Fatalf("closed audit query = %v, want 500", err)
	}

	// 管理员 + 成功 → 记录回显（含审计写入→读回闭环）
	if err := e.base.Audits.Insert(42, "a", "t", "r", "{}", ""); err != nil {
		t.Fatalf("insert audit: %v", err)
	}
	out, err := NewAdminLogic(ctx, e.base).ListAudit("")
	if err != nil || len(out.Items) == 0 || out.Items[0].Action != "a" {
		t.Fatalf("list audit = %+v/%v, want seeded action a", out, err)
	}
}
