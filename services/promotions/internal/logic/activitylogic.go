package logic

// GP-301 活动基础：活动配置（状态/起止时间时区/游戏/区服/渠道/人群/触发
// 条件/限次/预算/叠加/奖励）+ 发布版本快照（F18）。
//
// 生命周期：draft → published（版本快照落定）→ disabled → revise（回草稿）
// → publish（新版本）。**发布生成不可变版本快照**，运行时按 current_version
// 引用，旧版本保留可回溯；发布后不可改当前配置（须 revise 出新版本）。
// 服务端配置域（不信任客户端可信金额/规则；倍率/优惠计算归 GP-302/303/305）。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/promotions/internal/svc"
	"github.com/tappi/tappi/services/promotions/internal/types"
	"github.com/tappi/tappi/services/promotions/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// ActivityLogic 活动配置管理面。
type ActivityLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewActivityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ActivityLogic {
	return &ActivityLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func actor(ctx context.Context) (int64, string) {
	var userID int64
	var ip string
	if v, ok := ctx.Value("user_id").(int64); ok {
		userID = v
	}
	if v, ok := ctx.Value("client_ip").(string); ok {
		ip = v
	}
	return userID, ip
}

// validate 校验活动配置（服务端配置域，不信任客户端金额/规则）。
func validate(cfg *types.ActivityConfig) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return newAPIError(http.StatusBadRequest, "活动名不能为空")
	}
	if strings.TrimSpace(cfg.GameId) == "" {
		return newAPIError(http.StatusBadRequest, "游戏归属不能为空")
	}
	if strings.TrimSpace(cfg.Timezone) == "" {
		return newAPIError(http.StatusBadRequest, "时区不能为空")
	}
	start, end := strings.TrimSpace(cfg.StartAt), strings.TrimSpace(cfg.EndAt)
	if start == "" || end == "" {
		return newAPIError(http.StatusBadRequest, "起止时间不能为空")
	}
	if start > end {
		return newAPIError(http.StatusBadRequest, "开始时间不能晚于结束时间")
	}
	for _, s := range cfg.ServerIds {
		if s <= 0 {
			return newAPIError(http.StatusBadRequest, "区服标识必须为正整数")
		}
	}
	if cfg.Limits != nil {
		if v, ok := cfg.Limits["budget_cents"]; ok {
			if cents, ok := v.(float64); ok && cents < 0 {
				return newAPIError(http.StatusBadRequest, "预算不能为负")
			}
		}
	}
	return nil
}

// Create 建活动（落草稿）。
func (l *ActivityLogic) Create(req *types.CreateActivityRequest, authHeader string) (*types.ActivityResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	if err := validate(&req.ActivityConfig); err != nil {
		return nil, err
	}
	a := &model.Activity{
		Name: req.Name, GameId: req.GameId, Status: model.ActivityStatusDraft,
		Timezone: req.Timezone, StartAt: req.StartAt, EndAt: req.EndAt,
		Channel: req.Channel, Stackable: req.Stackable, StackGroup: req.StackGroup,
		CreatedBy: userID,
	}
	if a.ServerIds, err = marshalInts(req.ServerIds); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "区服配置不合法")
	}
	if a.Audience, err = marshalJSON(req.Audience); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "人群配置不合法")
	}
	if a.TriggerSpec, err = marshalJSON(req.Trigger); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "触发条件不合法")
	}
	if a.LimitsSpec, err = marshalJSON(req.Limits); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "限次配置不合法")
	}
	if a.RewardsSpec, err = marshalJSON(req.Rewards); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "奖励配置不合法")
	}
	if err := l.svcCtx.ActivityDB.Create(a); err != nil {
		l.Errorf("create activity failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "创建活动失败")
	}
	l.audit(a.Id, userID, role, "activity_create", ip, map[string]any{"name": a.Name, "game_id": a.GameId})
	return &types.ActivityResponse{Activity: toView(a)}, nil
}

// Update 更新活动配置（仅草稿；已发布/停用需 revise 出新版本）。
func (l *ActivityLogic) Update(req *types.UpdateActivityRequest, authHeader string) (*types.ActivityResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	if err := validate(&req.ActivityConfig); err != nil {
		return nil, err
	}
	existing, err := l.svcCtx.ActivityDB.Get(req.Id)
	if errors.Is(err, model.ErrActivityNotFound) {
		return nil, newAPIError(http.StatusNotFound, "活动不存在")
	}
	if err != nil {
		l.Errorf("get activity %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询活动失败")
	}
	if existing.Status != model.ActivityStatusDraft {
		return nil, newAPIError(http.StatusConflict, "仅草稿可编辑（已发布/停用请先 revise）")
	}
	a := &model.Activity{
		Id: req.Id, Name: req.Name, GameId: req.GameId, Status: model.ActivityStatusDraft,
		Timezone: req.Timezone, StartAt: req.StartAt, EndAt: req.EndAt,
		Channel: req.Channel, Stackable: req.Stackable, StackGroup: req.StackGroup,
	}
	if a.ServerIds, err = marshalInts(req.ServerIds); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "区服配置不合法")
	}
	if a.Audience, err = marshalJSON(req.Audience); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "人群配置不合法")
	}
	if a.TriggerSpec, err = marshalJSON(req.Trigger); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "触发条件不合法")
	}
	if a.LimitsSpec, err = marshalJSON(req.Limits); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "限次配置不合法")
	}
	if a.RewardsSpec, err = marshalJSON(req.Rewards); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "奖励配置不合法")
	}
	if err := l.svcCtx.ActivityDB.Update(a); err != nil {
		l.Errorf("update activity %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "更新活动失败")
	}
	got, _ := l.svcCtx.ActivityDB.Get(req.Id)
	l.audit(req.Id, userID, role, "activity_update", ip, map[string]any{"name": a.Name})
	return &types.ActivityResponse{Activity: toView(got)}, nil
}

// Publish 发布：落不可变版本快照并推进 current_version。
func (l *ActivityLogic) Publish(req *types.ActivityPathRequest, authHeader string) (*types.ActivityResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	a, err := l.svcCtx.ActivityDB.Get(req.Id)
	if errors.Is(err, model.ErrActivityNotFound) {
		return nil, newAPIError(http.StatusNotFound, "活动不存在")
	}
	if err != nil {
		l.Errorf("get activity %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询活动失败")
	}
	if a.Status != model.ActivityStatusDraft {
		return nil, newAPIError(http.StatusConflict, "仅草稿可发布（已发布请先 disable 或 revise）")
	}
	version, err := l.svcCtx.ActivityDB.NextVersion(req.Id)
	if err != nil {
		l.Errorf("next version %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "生成版本失败")
	}
	snapshot, _ := json.Marshal(map[string]any{
		"name": a.Name, "game_id": a.GameId, "timezone": a.Timezone,
		"start_at": a.StartAt, "end_at": a.EndAt, "server_ids": unmarshalInts(a.ServerIds),
		"channel": a.Channel, "audience": unmarshalJSON(a.Audience),
		"trigger": unmarshalJSON(a.TriggerSpec), "limits": unmarshalJSON(a.LimitsSpec),
		"stackable": a.Stackable, "stack_group": a.StackGroup,
		"rewards": unmarshalJSON(a.RewardsSpec),
	})
	if err := l.svcCtx.ActivityDB.PublishSnapshot(req.Id, version, string(snapshot), userID); err != nil {
		l.Errorf("publish activity %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "发布活动失败")
	}
	got, _ := l.svcCtx.ActivityDB.Get(req.Id)
	l.audit(req.Id, userID, role, "activity_publish", ip, map[string]any{"version": version})
	return &types.ActivityResponse{Activity: toView(got)}, nil
}

// Disable 停用（仅已发布）。
func (l *ActivityLogic) Disable(req *types.ActivityPathRequest, authHeader string) (*types.ActivityResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	a, err := l.svcCtx.ActivityDB.Get(req.Id)
	if errors.Is(err, model.ErrActivityNotFound) {
		return nil, newAPIError(http.StatusNotFound, "活动不存在")
	}
	if err != nil {
		l.Errorf("get activity %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询活动失败")
	}
	if a.Status != model.ActivityStatusPublished {
		return nil, newAPIError(http.StatusConflict, "仅已发布活动可停用")
	}
	if err := l.svcCtx.ActivityDB.SetStatus(req.Id, model.ActivityStatusDisabled); err != nil {
		l.Errorf("disable activity %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "停用活动失败")
	}
	got, _ := l.svcCtx.ActivityDB.Get(req.Id)
	l.audit(req.Id, userID, role, "activity_disable", ip, nil)
	return &types.ActivityResponse{Activity: toView(got)}, nil
}

// Revise 回草稿（已发布/停用 → 草稿；下次发布新版本）。
func (l *ActivityLogic) Revise(req *types.ActivityPathRequest, authHeader string) (*types.ActivityResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	a, err := l.svcCtx.ActivityDB.Get(req.Id)
	if errors.Is(err, model.ErrActivityNotFound) {
		return nil, newAPIError(http.StatusNotFound, "活动不存在")
	}
	if err != nil {
		l.Errorf("get activity %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询活动失败")
	}
	if a.Status == model.ActivityStatusDraft {
		return nil, newAPIError(http.StatusConflict, "已是草稿")
	}
	if err := l.svcCtx.ActivityDB.SetStatus(req.Id, model.ActivityStatusDraft); err != nil {
		l.Errorf("revise activity %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "回草稿失败")
	}
	got, _ := l.svcCtx.ActivityDB.Get(req.Id)
	l.audit(req.Id, userID, role, "activity_revise", ip, map[string]any{"from": a.Status})
	return &types.ActivityResponse{Activity: toView(got)}, nil
}

// List 活动列表（game_id/status 可筛）。
func (l *ActivityLogic) List(req *types.ActivityListRequest, authHeader string) (*types.ActivityListResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.ActivityDB.List(req.GameId, req.Status, 50, 0)
	if err != nil {
		l.Errorf("list activities failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询活动失败")
	}
	items := make([]types.ActivityView, 0, len(rows))
	for _, r := range rows {
		items = append(items, toView(r))
	}
	l.audit(0, userID, role, "activity_list", ip, map[string]string{"game_id": req.GameId, "status": req.Status})
	return &types.ActivityListResponse{Items: items}, nil
}

// Get 活动详情。
func (l *ActivityLogic) Get(req *types.ActivityPathRequest, authHeader string) (*types.ActivityResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	a, err := l.svcCtx.ActivityDB.Get(req.Id)
	if errors.Is(err, model.ErrActivityNotFound) {
		return nil, newAPIError(http.StatusNotFound, "活动不存在")
	}
	if err != nil {
		l.Errorf("get activity %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询活动失败")
	}
	l.audit(req.Id, userID, role, "activity_view", ip, nil)
	return &types.ActivityResponse{Activity: toView(a)}, nil
}

// Versions 活动版本列表（新→旧，快照不可变）。
func (l *ActivityLogic) Versions(req *types.ActivityPathRequest, authHeader string) (*types.VersionListResponse, error) {
	_, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	if _, err := l.svcCtx.ActivityDB.Get(req.Id); errors.Is(err, model.ErrActivityNotFound) {
		return nil, newAPIError(http.StatusNotFound, "活动不存在")
	}
	rows, err := l.svcCtx.ActivityDB.ListVersions(req.Id)
	if err != nil {
		l.Errorf("list versions %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询版本失败")
	}
	items := make([]types.VersionView, 0, len(rows))
	for _, v := range rows {
		items = append(items, types.VersionView{
			Id: v.Id, ActivityId: v.ActivityId, Version: v.Version,
			Snapshot: unmarshalJSON(v.Snapshot), PublishedBy: v.PublishedBy, PublishedAt: v.PublishedAt,
		})
	}
	return &types.VersionListResponse{Items: items}, nil
}

// ListAudit 活动审计列表。
func (l *ActivityLogic) ListAudit(activityID int64, authHeader string) (*types.AuditListResponse, error) {
	if _, err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.ActivityDB.ListAudit(activityID, 100)
	if err != nil {
		l.Errorf("list audit failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询审计失败")
	}
	items := make([]types.AuditView, 0, len(rows))
	for _, a := range rows {
		items = append(items, types.AuditView{
			Id: a.Id, ActivityId: a.ActivityId, ActorId: a.ActorId, ActorRole: a.ActorRole,
			Action: a.Action, Detail: a.Detail, Ip: a.Ip, CreatedAt: a.CreatedAt,
		})
	}
	return &types.AuditListResponse{Items: items}, nil
}

func (l *ActivityLogic) audit(activityID, actorID int64, role, action, ip string, detail any) {
	var detailJSON string
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON = string(b)
		}
	}
	_ = l.svcCtx.ActivityDB.LogAudit(&model.ActivityAuditLog{
		ActivityId: activityID, ActorId: actorID, ActorRole: role,
		Action: action, Detail: detailJSON, Ip: ip,
	})
}

func marshalJSON(v map[string]any) (string, error) {
	if v == nil {
		return "{}", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalJSON(s string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return map[string]any{}
	}
	return m
}

func marshalInts(v []int64) (string, error) {
	if len(v) == 0 {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalInts(s string) []int64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var v []int64
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil
	}
	return v
}

func toView(a *model.Activity) types.ActivityView {
	return types.ActivityView{
		Id: a.Id, Name: a.Name, GameId: a.GameId, Status: a.Status,
		Timezone: a.Timezone, StartAt: a.StartAt, EndAt: a.EndAt,
		ServerIds: unmarshalInts(a.ServerIds), Channel: a.Channel,
		Audience: unmarshalJSON(a.Audience), Trigger: unmarshalJSON(a.TriggerSpec),
		Limits: unmarshalJSON(a.LimitsSpec), Stackable: a.Stackable, StackGroup: a.StackGroup,
		Rewards: unmarshalJSON(a.RewardsSpec), CurrentVersion: a.CurrentVersion,
		CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}
