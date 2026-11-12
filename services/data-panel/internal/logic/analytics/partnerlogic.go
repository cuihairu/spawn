package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// PartnerLogic 合作方自助数据面（GP-404 F12）：partner 角色 + game-catalog
// 绑定的 partner_id，数据按 partner_id 行级隔离（只统计本人名下事件）。
// 角色权威在 game-catalog /me（data-panel 不复制角色数据）；目录不可达
// fail-closed——无法判定归属就不返回数据。
type PartnerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPartnerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PartnerLogic {
	return &PartnerLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// Query 期间内本人名下事件的按类型聚合（注册/活跃/下载/付费）。
// 行级隔离键优先取 game-catalog 名下游戏 id（game→partner 归属的单一事实源，
// 注册/活跃/下载等未带 partner_id 的平台级事件也能按游戏正确归属）；名下无
// 游戏时回落 partner_id 命中。
func (l *PartnerLogic) Query(req *types.AnalyticsPartnerReq) (*types.AnalyticsPartnerResp, error) {
	partnerID, authHeader, err := l.requirePartner()
	if err != nil {
		return nil, err
	}
	from, to, err := normalizeBounds(req.From, req.To)
	if err != nil {
		return nil, err
	}
	gameIds := l.ownGameIDs(authHeader, partnerID)
	rows, err := l.svcCtx.AnalyticsSvc.PartnerOverview(from, to, partnerID, gameIds)
	if err != nil {
		return nil, httperr.Internal("聚合合作方数据失败")
	}
	items := make([]types.AnalyticsTypeCount, 0, len(rows))
	for _, r := range rows {
		items = append(items, types.AnalyticsTypeCount{
			EventType: r.EventType, Events: r.Events, Users: r.Users, AmountCents: r.Amount,
		})
	}
	return &types.AnalyticsPartnerResp{From: from, To: to, PartnerId: partnerID, Items: items}, nil
}

// requirePartner 角色守卫：必须是 game-catalog 的 partner 角色且已绑定
// 合作方 id（非 partner 或未绑定 → 403；目录不可达 → 502）。返回 authHeader
// 供后续 /partner/games 复用（同一权威来源，不二次校验令牌）。
func (l *PartnerLogic) requirePartner() (int64, string, error) {
	authHeader := ""
	if v, ok := l.ctx.Value("auth_header").(string); ok {
		authHeader = v
	}
	role, partnerID, err := partnerRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return 0, "", httperr.New(http.StatusBadGateway, 50200, "角色服务不可用")
	}
	if role != "partner" {
		return 0, "", httperr.Forbidden("partner only")
	}
	if partnerID <= 0 {
		return 0, "", httperr.Forbidden("当前账号未绑定合作方")
	}
	return partnerID, authHeader, nil
}

// ownGameIDs 取本人名下游戏 id（game-catalog 归属权威；空 = 名下无游戏）。
// 查询失败按空返回（回落 partner_id 口径），不阻断自助查询。
func (l *PartnerLogic) ownGameIDs(authHeader string, partnerID int64) []string {
	cfg := l.svcCtx.Config.GameCatalog
	if cfg.BaseURL == "" {
		return nil
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5000
	}
	hc := &http.Client{Timeout: time.Millisecond * time.Duration(timeout)}
	req, err := http.NewRequestWithContext(l.ctx, http.MethodGet,
		strings.TrimRight(cfg.BaseURL, "/")+"/partner/games", nil)
	if err != nil {
		return nil
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := hc.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil
	}
	var list struct {
		Games []struct {
			GameId string `json:"game_id"`
			Id     string `json:"id"`
		} `json:"games"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil
	}
	out := make([]string, 0, len(list.Games))
	for _, g := range list.Games {
		id := strings.TrimSpace(g.GameId)
		if id == "" {
			id = strings.TrimSpace(g.Id)
		}
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// partnerRole 经 game-catalog GET /me 读平台角色与合作方归属（单一事实源）。
func partnerRole(ctx context.Context, svcCtx *svc.ServiceContext, authHeader string) (string, int64, error) {
	cfg := svcCtx.Config.GameCatalog
	if cfg.BaseURL == "" {
		return "", 0, fmt.Errorf("game catalog base url is empty")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5000
	}
	hc := &http.Client{Timeout: time.Millisecond * time.Duration(timeout)}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(cfg.BaseURL, "/")+"/me", nil)
	if err != nil {
		return "", 0, err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("game-catalog unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("game-catalog /me status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", 0, err
	}
	var me struct {
		Role      string `json:"role"`
		PartnerId int64  `json:"partner_id"`
	}
	if err := json.Unmarshal(raw, &me); err != nil {
		return "", 0, fmt.Errorf("decode /me: %w", err)
	}
	return me.Role, me.PartnerId, nil
}
