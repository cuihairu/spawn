package logic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/tappi/tappi/services/courier-adapter/internal/svc"
	"github.com/tappi/tappi/services/courier-adapter/internal/types"
	"github.com/tappi/tappi/services/courier-adapter/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type AdminLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAdminLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AdminLogic {
	return &AdminLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

var envRe = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

// UpsertMapping 映射写入（operator/platform_admin）。scope.md：
// (courier_game_id, env) 全局唯一；env 按字符串处理（注册表制可扩展），
// 此处仅做形态校验（小写字母数字 ≤16）。
func (l *AdminLogic) UpsertMapping(req *types.UpsertMappingRequest, authHeader string, userID int64) error {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return err
	}
	if strings.TrimSpace(req.CourierGameId) == "" {
		return newAPIError(http.StatusBadRequest, "courierGameId 不能为空")
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "prod"
	}
	if !envRe.MatchString(env) {
		return newAPIError(http.StatusBadRequest, "env 需为 ≤16 位小写字母数字（如 prod/staging/dev）")
	}
	m := &model.GameMapping{
		SpawnGameId:   req.Id,
		CourierGameId: strings.TrimSpace(req.CourierGameId),
		Env:           env,
		PartnerId:     req.PartnerId,
		Capabilities: model.Capabilities{
			Login:    req.Capabilities.Login,
			Payment:  req.Capabilities.Payment,
			Download: req.Capabilities.Download,
		},
	}
	if err := l.svcCtx.Mappings.Upsert(m); err != nil {
		l.Errorf("upsert mapping failed (game %s): %v", req.Id, err)
		return newAPIError(http.StatusInternalServerError, "保存映射失败")
	}
	if err := l.svcCtx.Audits.Insert(userID, "courier_mapping_upsert", "mapping", req.Id,
		fmtMappingDetail(m), clientIP(l.ctx)); err != nil {
		l.Errorf("audit mapping upsert failed: %v", err)
	}
	return nil
}

// GetMapping 映射读回（operator/platform_admin；玩家侧经能力端点感知）。
func (l *AdminLogic) GetMapping(spawnGameID, authHeader string) (*types.MappingResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	m, err := l.svcCtx.Mappings.Get(spawnGameID)
	if err != nil {
		if errors.Is(err, model.ErrMappingNotFound) {
			return nil, newAPIError(http.StatusNotFound, "游戏未接入 Courier")
		}
		return nil, newAPIError(http.StatusInternalServerError, "查询映射失败")
	}
	return &types.MappingResponse{
		SpawnGameId:   m.SpawnGameId,
		CourierGameId: m.CourierGameId,
		Env:           m.Env,
		PartnerId:     m.PartnerId,
		Capabilities: types.Capabilities{
			Login:    m.Capabilities.Login,
			Payment:  m.Capabilities.Payment,
			Download: m.Capabilities.Download,
		},
	}, nil
}

// ListAudit 最近审计（operator/platform_admin）。
func (l *AdminLogic) ListAudit(authHeader string) (*types.ListAuditResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	items, err := l.svcCtx.Audits.ListRecent(50)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询审计失败")
	}
	out := &types.ListAuditResponse{Items: make([]types.AuditItem, 0, len(items))}
	for _, e := range items {
		out.Items = append(out.Items, types.AuditItem{
			Id: e.ID, ActorId: e.ActorID, Action: e.Action, ResourceType: e.ResourceType,
			ResourceId: e.ResourceID, Detail: e.Detail, Ip: e.IP, CreatedAt: e.CreatedAt,
		})
	}
	return out, nil
}

func fmtMappingDetail(m *model.GameMapping) string {
	return fmt.Sprintf(`{"courierGameId":%q,"env":%q,"partnerId":%d,`+
		`"capabilities":{"login":%t,"payment":%t,"download":%t}}`,
		m.CourierGameId, m.Env, m.PartnerId,
		m.Capabilities.Login, m.Capabilities.Payment, m.Capabilities.Download)
}
