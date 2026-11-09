package logic

// GP-302 倍率赠送评估：payments 在订单 PAID 后推送订单事实（S2S 签名），
// 本服务按已发布版本快照匹配活动、按倍率语义（total/extra）服务端计算
// 基础量/额外量/总量，落奖励账本（(order_id, activity_id) 唯一幂等）。
// 基础量来自服务端商品配置随订单事实传入，不信任客户端；赠币不进分成
// 基数（GP-205 实付口径不变）。

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/tappi/tappi/services/promotions/internal/svc"
	"github.com/tappi/tappi/services/promotions/internal/types"
	"github.com/tappi/tappi/services/promotions/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// 评估请求头：X-Reward-Signature = hex(HMAC-SHA256(secret, rawBody))；
// 验签先于解析；未配置密钥 fail-closed（403，与 payments 发货接缝同款）。
const rewardSignatureHeader = "X-Reward-Signature"

// RewardLogic 奖励评估逻辑。
type RewardLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewRewardLogic 构造。
func NewRewardLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RewardLogic {
	return &RewardLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// Evaluate 评估一笔订单的活动奖励（幂等：同订单重放返回已发放事实，
// 不重复发放）。
func (l *RewardLogic) Evaluate(rawBody []byte, signature string) (*types.RewardEvaluateResponse, error) {
	secret := l.svcCtx.Config.Reward.Secret
	if strings.TrimSpace(secret) == "" {
		return nil, newAPIError(http.StatusForbidden, "奖励评估未配置密钥（fail-closed）")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(strings.ToLower(strings.TrimSpace(signature)))) {
		return nil, newAPIError(http.StatusForbidden, "评估签名无效")
	}

	var req types.RewardEvaluateRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "评估请求解析失败")
	}
	if strings.TrimSpace(req.OrderId) == "" {
		return nil, newAPIError(http.StatusBadRequest, "订单号不能为空")
	}
	if strings.TrimSpace(req.GameId) == "" {
		return nil, newAPIError(http.StatusBadRequest, "游戏归属不能为空")
	}
	if req.PaidCents < 0 || req.BaseQuantity < 0 {
		return nil, newAPIError(http.StatusBadRequest, "实付金额/基础量不能为负")
	}
	paidAt := strings.TrimSpace(req.PaidAt)
	if paidAt == "" {
		paidAt = time.Now().UTC().Format(time.RFC3339)
	}

	resp := &types.RewardEvaluateResponse{OrderId: req.OrderId}

	// 幂等重放：该订单已有账本行时直接返回已发放事实（不重复发放）
	existing, err := l.svcCtx.RewardDB.ListGrants(0, req.OrderId, "", "")
	if err != nil {
		l.Errorf("list grants order=%s failed: %v", req.OrderId, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询奖励账本失败")
	}
	if len(existing) > 0 {
		resp.Replay = true
		for i := range existing {
			resp.Grants = append(resp.Grants, toGrantView(&existing[i]))
		}
		l.audit(req, "reward_evaluate_replay", map[string]any{"grants": len(existing)})
		return resp, nil
	}

	// 命中：已发布版本快照（不读可变草稿行）
	published, err := l.svcCtx.ActivityDB.ListPublishedSnapshots(req.GameId)
	if err != nil {
		l.Errorf("list published activities game=%s failed: %v", req.GameId, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询活动失败")
	}
	skipped := map[int64]string{}
	var cands []*candidate
	for _, p := range published {
		cfg, err := parseSnapshot(p.Snapshot)
		if err != nil {
			l.Errorf("parse snapshot activity=%d: %v", p.ActivityId, err)
			continue
		}
		if err := matchTarget(cfg, req.GameId, req.ServerId, req.Channel, req.AudienceType); err != nil {
			continue
		}
		if !periodCovers(cfg, paidAt) {
			continue
		}
		spec, err := ParseMultiplierSpec(snapshotMap(cfg, "rewards"))
		if err != nil || spec == nil {
			continue
		}
		trg, err := parseTrigger(snapshotMap(cfg, "trigger"))
		if err != nil {
			skipped[p.ActivityId] = "trigger-invalid"
			continue
		}
		if req.PaidCents < trg.MinAmountCents {
			continue
		}
		lim, err := parseLimits(snapshotMap(cfg, "limits"))
		if err != nil {
			skipped[p.ActivityId] = "limits-invalid"
			continue
		}
		cands = append(cands, &candidate{
			ActivityId: p.ActivityId, Version: p.Version, Config: cfg,
			Spec: spec, Trigger: trg, Limits: lim,
			Stackable:     snapshotBool(cfg, "stackable"),
			StackGroup:    snapshotString(cfg, "stack_group"),
			BaseQuantity:  req.BaseQuantity,
			ExtraQuantity: spec.bonusQuantity(req.BaseQuantity),
		})
	}

	winners, excluded := resolveStack(cands)
	for id, reason := range excluded {
		skipped[id] = reason
	}
	for _, w := range winners {
		// 零赠送不落账本（无意义的空发放）
		if w.ExtraQuantity <= 0 {
			skipped[w.ActivityId] = "zero-bonus"
			continue
		}
		// 限次消费（per_user_count / global_count；budget 扣减不在本项）
		if w.Limits.PerUserCount > 0 {
			n, err := l.svcCtx.RewardDB.CountGrantsByUser(w.ActivityId, req.AccountId)
			if err != nil {
				l.Errorf("count user grants activity=%d failed: %v", w.ActivityId, err)
				return nil, newAPIError(http.StatusInternalServerError, "限次查询失败")
			}
			if n >= w.Limits.PerUserCount {
				skipped[w.ActivityId] = "per-user-limit-reached"
				continue
			}
		}
		if w.Limits.GlobalCount > 0 {
			n, err := l.svcCtx.RewardDB.CountGrantsByActivity(w.ActivityId)
			if err != nil {
				l.Errorf("count activity grants failed: %v", w.ActivityId, err)
				return nil, newAPIError(http.StatusInternalServerError, "限次查询失败")
			}
			if n >= w.Limits.GlobalCount {
				skipped[w.ActivityId] = "global-limit-reached"
				continue
			}
		}
		g := &model.RewardGrant{
			OrderId: req.OrderId, ActivityId: w.ActivityId, ActivityVersion: w.Version,
			GameId: req.GameId, ServerId: req.ServerId, RoleId: req.RoleId,
			AccountId: req.AccountId, SkuId: req.SkuId,
			PaidCents: req.PaidCents, Currency: req.Currency,
			BaseQuantity:  req.BaseQuantity,
			ExtraQuantity: w.ExtraQuantity,
			TotalQuantity: req.BaseQuantity + w.ExtraQuantity,
			RateMode:      w.Spec.Mode, RateBp: w.Spec.RateBp,
			Status:    model.RewardStatusGranted,
			Snapshot:  buildGrantSnapshot(req, paidAt, w),
			GrantedAt: time.Now().UTC().Format(time.RFC3339),
		}
		created, err := l.svcCtx.RewardDB.InsertGrant(g)
		if err != nil {
			l.Errorf("insert grant order=%s activity=%d failed: %v", req.OrderId, w.ActivityId, err)
			return nil, newAPIError(http.StatusInternalServerError, "落奖励账本失败")
		}
		if created {
			resp.Grants = append(resp.Grants, toGrantView(g))
		} else {
			// 并发同订单重放：返回已落库事实（幂等）
			row, err := l.svcCtx.RewardDB.GetGrant(req.OrderId, w.ActivityId)
			if err != nil {
				l.Errorf("get grant order=%s activity=%d failed: %v", req.OrderId, w.ActivityId, err)
				return nil, newAPIError(http.StatusInternalServerError, "查询奖励账本失败")
			}
			resp.Grants = append(resp.Grants, toGrantView(row))
		}
	}
	for id, reason := range skipped {
		resp.Skipped = append(resp.Skipped, types.RewardSkip{ActivityId: id, Reason: reason})
	}
	l.audit(req, "reward_evaluate", map[string]any{
		"grants": len(resp.Grants), "skipped": len(resp.Skipped),
		"paid_cents": req.PaidCents, "base_quantity": req.BaseQuantity,
	})
	return resp, nil
}

// buildGrantSnapshot 固化订单+奖励快照（命中的活动版本、倍率语义、计算
// 明细、目标）——后改活动/商品不改变旧账本。
func buildGrantSnapshot(req types.RewardEvaluateRequest, paidAt string, w *candidate) string {
	formulas := map[string]string{
		ModeTotal: "extra = base x (rate_bp - 10000) / 10000",
		ModeExtra: "extra = base x rate_bp / 10000",
	}
	snap, _ := json.Marshal(map[string]any{
		"activity": map[string]any{
			"id": w.ActivityId, "version": w.Version,
			"name": snapshotString(w.Config, "name"),
		},
		"rule": map[string]any{
			"mode": w.Spec.Mode, "rate_bp": w.Spec.RateBp,
			"formula": formulas[w.Spec.Mode],
		},
		"computation": map[string]any{
			"base_quantity":  req.BaseQuantity,
			"extra_quantity": w.ExtraQuantity,
			"total_quantity": req.BaseQuantity + w.ExtraQuantity,
		},
		"order": map[string]any{
			"order_id": req.OrderId, "account_id": req.AccountId,
			"server_id": req.ServerId, "role_id": req.RoleId,
			"sku_id": req.SkuId, "product_id": req.ProductId,
			"channel": req.Channel, "paid_cents": req.PaidCents,
			"currency": req.Currency, "paid_at": paidAt,
		},
		"stack": map[string]any{
			"stackable": w.Stackable, "stack_group": w.StackGroup,
		},
	})
	return string(snap)
}

// ListGrants 管理面查询发放账本（角色经 /me 权威判定）。
func (l *RewardLogic) ListGrants(req *types.RewardGrantListRequest, authHeader string) (*types.RewardGrantListResponse, error) {
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	userID, ip := actor(l.ctx)
	rows, err := l.svcCtx.RewardDB.ListGrants(req.ActivityId, req.OrderId, req.GameId, req.AccountId)
	if err != nil {
		l.Errorf("list reward grants failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询奖励账本失败")
	}
	views := make([]types.RewardGrantView, 0, len(rows))
	for i := range rows {
		views = append(views, toGrantView(&rows[i]))
	}
	detail, _ := json.Marshal(map[string]any{
		"activity_id": req.ActivityId, "order_id": req.OrderId,
		"game_id": req.GameId, "account_id": req.AccountId, "count": len(views),
	})
	_ = l.svcCtx.ActivityDB.LogAudit(&model.ActivityAuditLog{
		ActivityId: 0, ActorId: userID, ActorRole: role,
		Action: "reward_grant_list", Detail: string(detail), Ip: ip,
	})
	return &types.RewardGrantListResponse{Grants: views}, nil
}

func toGrantView(g *model.RewardGrant) types.RewardGrantView {
	return types.RewardGrantView{
		Id: g.Id, OrderId: g.OrderId, ActivityId: g.ActivityId,
		ActivityVersion: g.ActivityVersion, GameId: g.GameId,
		ServerId: g.ServerId, RoleId: g.RoleId, AccountId: g.AccountId,
		SkuId: g.SkuId, PaidCents: g.PaidCents, Currency: g.Currency,
		BaseQuantity: g.BaseQuantity, ExtraQuantity: g.ExtraQuantity,
		TotalQuantity: g.TotalQuantity, RateMode: g.RateMode, RateBp: g.RateBp,
		Status: g.Status, Snapshot: json.RawMessage(g.Snapshot),
		GrantedAt: g.GrantedAt,
	}
}

// audit 记评估审计（activity_id=0 的全局审计，detail 带订单可检索）。
func (l *RewardLogic) audit(req types.RewardEvaluateRequest, action string, detail map[string]any) {
	userID, ip := actor(l.ctx)
	detail["order_id"] = req.OrderId
	detail["game_id"] = req.GameId
	detailJSON, _ := json.Marshal(detail)
	_ = l.svcCtx.ActivityDB.LogAudit(&model.ActivityAuditLog{
		ActivityId: 0, ActorId: userID, ActorRole: "internal",
		Action: action, Detail: string(detailJSON), Ip: ip,
	})
}
