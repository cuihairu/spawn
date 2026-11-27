package handler

// GP-202 F15 商户归属 S2S 端点覆盖：/internal/games/:id/ownership 供
// payments 商品配置核验 partner_id（game-catalog 为权威源）。令牌未配置/
// 缺失/错误恒 403；正确令牌返回 active 归属；未绑定 404；仓储故障 500。

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/model"
)

func internalOwnershipRequest(t *testing.T, base, id, token string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+"/internal/games/"+id+"/ownership", nil)
	if token != "" {
		req.Header.Set("X-Internal-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("internal ownership request: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, readAll(t, resp)
}

func TestRoutes_InternalGameOwnership(t *testing.T) {
	const s2sToken = "ownership-s2s-token"
	base, sign, svcCtx := newAuthedTestServerSvc(t, func(c *config.Config) { c.Internal.Token = s2sToken })

	// 先指派归属：game-elden-ring → 新建合作方
	admin := sign(claimsFor(1, "admin"))
	code, body := doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"F15 归属合作方"}`)
	if code != http.StatusOK {
		t.Fatalf("create partner = %d body=%s", code, body)
	}
	var partner struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &partner)
	code, body = doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/ownership", admin,
		`{"partner_id":`+jsonNumber(partner.Id)+`}`)
	if code != http.StatusOK {
		t.Fatalf("assign ownership = %d body=%s", code, body)
	}

	// 缺令牌/错令牌 403；令牌未配置时即便头正确也 403
	if code, _ := internalOwnershipRequest(t, base, "game-elden-ring", ""); code != http.StatusForbidden {
		t.Fatalf("no token = %d, want 403", code)
	}
	if code, _ := internalOwnershipRequest(t, base, "game-elden-ring", "wrong"); code != http.StatusForbidden {
		t.Fatalf("wrong token = %d, want 403", code)
	}
	unconfigured, _ := newAuthedTestServer(t)
	if code, _ := internalOwnershipRequest(t, unconfigured, "game-elden-ring", s2sToken); code != http.StatusForbidden {
		t.Fatalf("token not configured = %d, want 403", code)
	}

	// 正确令牌 → 200，回显权威归属（partner_id/status）
	code, body = internalOwnershipRequest(t, base, "game-elden-ring", s2sToken)
	if code != http.StatusOK {
		t.Fatalf("ownership = %d body=%s", code, body)
	}
	var got struct {
		GameId    string `json:"game_id"`
		PartnerId int64  `json:"partner_id"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil || got.GameId != "game-elden-ring" ||
		got.PartnerId != partner.Id || got.Status != model.OwnershipActive {
		t.Fatalf("ownership body = %s err=%v", body, err)
	}

	// 未绑定游戏 404（归属缺失）
	if code, _ := internalOwnershipRequest(t, base, "game-noowner", s2sToken); code != http.StatusNotFound {
		t.Fatalf("missing ownership = %d, want 404", code)
	}

	// 仓储查询失败 → 500（fail-closed，payments 不能把错误当无归属）
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:      svcCtx.AdminRepository,
		getOwnershipErr: errors.New("ownership store down"),
	}
	if code, _ := internalOwnershipRequest(t, base, "game-elden-ring", s2sToken); code != http.StatusInternalServerError {
		t.Fatalf("store failure = %d, want 500", code)
	}
}

// 守卫必须在 body/path 解析之前：缺失令牌也不能绕过 403（S2S 端点不进
// 网关，令牌是第一道门）。
func TestRoutes_InternalGameOwnershipGuardBeforeParse(t *testing.T) {
	const s2sToken = "ownership-s2s-token"
	base, _ := newAuthedTestServerConfig(t, func(c *config.Config) { c.Internal.Token = s2sToken })
	if code, _ := internalOwnershipRequest(t, base, "any-game", ""); code != http.StatusForbidden {
		t.Fatalf("guard before parse = %d, want 403", code)
	}
}
