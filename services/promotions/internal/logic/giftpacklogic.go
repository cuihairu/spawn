package logic

// GP-304 礼包域：礼包展示（配置 + 库存 + 已领）、码库存（导入/签发/核销）、
// 领取记录（per_user 限领 + 幂等键）。游戏内兑换边界：领取产出领取记录与
// （码包）核销码；实际内容发放归游戏服接缝（GP-203 同款签名回调，游戏服
// 核销后回调 /internal/gift-pack-redemptions 记 used）或 Courier /v1/cdk
// （Frozen v1，按账号+码幂等）——Courier F62 已落地但 Spawn→Courier cdk/mail
// 客户端接线为独立步骤，见 todo（未实现能力不标已完成）。
//
// 管理面角色经 game-catalog /me 权威判定；玩家侧仅平台 JWT。限领并发安全：
// 槽位 claim_index=现有数+1，UNIQUE(pack_id, account_id, claim_index) 保证
// 不超额；幂等锚 UNIQUE(pack_id, account_id, request_id) 保证重提交不重复。

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tappi/tappi/services/promotions/internal/svc"
	"github.com/tappi/tappi/services/promotions/internal/types"
	"github.com/tappi/tappi/services/promotions/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// GiftPackLogic 礼包逻辑。
type GiftPackLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGiftPackLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GiftPackLogic {
	return &GiftPackLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// validatePack 草稿校验（服务端配置域）。
func validatePack(cfg *types.GiftPackConfig) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return newAPIError(http.StatusBadRequest, "礼包名不能为空")
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
	if cfg.PerUserLimit <= 0 {
		return newAPIError(http.StatusBadRequest, "per_user_limit 必须为正整数")
	}
	for _, s := range cfg.ServerIds {
		if s <= 0 {
			return newAPIError(http.StatusBadRequest, "区服标识必须为正整数")
		}
	}
	if len(cfg.Content) == 0 {
		return newAPIError(http.StatusBadRequest, "礼包内容不能为空")
	}
	return nil
}

func (l *GiftPackLogic) Create(req *types.CreateGiftPackRequest, authHeader string) (*types.GiftPackResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	if err := validatePack(&req.GiftPackConfig); err != nil {
		return nil, err
	}
	p := &model.GiftPack{
		Name: req.Name, GameId: req.GameId, Description: req.Description,
		Status: model.GiftPackStatusDraft, Timezone: req.Timezone,
		StartAt: req.StartAt, EndAt: req.EndAt, Channel: req.Channel,
		PerUserLimit: req.PerUserLimit, CodeRequired: req.CodeRequired,
		CreatedBy: userID,
	}
	if p.ServerIds, err = marshalInts(req.ServerIds); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "区服配置不合法")
	}
	if p.Audience, err = marshalJSON(req.Audience); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "人群配置不合法")
	}
	if p.Content, err = marshalJSON(req.Content); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "礼包内容不合法")
	}
	if err := l.svcCtx.GiftPackDB.Create(p); err != nil {
		l.Errorf("create gift pack failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "创建礼包失败")
	}
	l.packAudit(p.Id, userID, role, "gift_pack_create", ip, map[string]any{"name": p.Name})
	got, _ := l.svcCtx.GiftPackDB.Get(p.Id)
	return &types.GiftPackResponse{GiftPack: toPackView(got)}, nil
}

func (l *GiftPackLogic) Update(req *types.UpdateGiftPackRequest, authHeader string) (*types.GiftPackResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	if err := validatePack(&req.GiftPackConfig); err != nil {
		return nil, err
	}
	existing, err := l.svcCtx.GiftPackDB.Get(req.Id)
	if errors.Is(err, model.ErrGiftPackNotFound) {
		return nil, newAPIError(http.StatusNotFound, "礼包不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询礼包失败")
	}
	if existing.Status != model.GiftPackStatusDraft {
		return nil, newAPIError(http.StatusConflict, "仅草稿可编辑（已发布/停用请先 revise）")
	}
	p := &model.GiftPack{
		Id: req.Id, Name: req.Name, GameId: req.GameId, Description: req.Description,
		Timezone: req.Timezone, StartAt: req.StartAt, EndAt: req.EndAt,
		Channel: req.Channel, PerUserLimit: req.PerUserLimit, CodeRequired: req.CodeRequired,
	}
	if p.ServerIds, err = marshalInts(req.ServerIds); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "区服配置不合法")
	}
	if p.Audience, err = marshalJSON(req.Audience); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "人群配置不合法")
	}
	if p.Content, err = marshalJSON(req.Content); err != nil {
		return nil, newAPIError(http.StatusBadRequest, "礼包内容不合法")
	}
	if err := l.svcCtx.GiftPackDB.Update(p); err != nil {
		l.Errorf("update gift pack %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "更新礼包失败")
	}
	l.packAudit(req.Id, userID, role, "gift_pack_update", ip, map[string]any{"name": p.Name})
	got, _ := l.svcCtx.GiftPackDB.Get(req.Id)
	return &types.GiftPackResponse{GiftPack: toPackView(got)}, nil
}

func (l *GiftPackLogic) Publish(req *types.GiftPackPathRequest, authHeader string) (*types.GiftPackResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	if err := l.requireStatus(req.Id, model.GiftPackStatusDraft); err != nil {
		return nil, err
	}
	// 码包发布前须有可用码（否则发即领完，无意义）
	if err := l.ensureStock(req.Id); err != nil {
		return nil, err
	}
	if err := l.svcCtx.GiftPackDB.SetStatus(req.Id, model.GiftPackStatusPublished); err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "发布礼包失败")
	}
	l.packAudit(req.Id, userID, role, "gift_pack_publish", ip, nil)
	got, _ := l.svcCtx.GiftPackDB.Get(req.Id)
	return &types.GiftPackResponse{GiftPack: toPackView(got)}, nil
}

func (l *GiftPackLogic) Disable(req *types.GiftPackPathRequest, authHeader string) (*types.GiftPackResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	if err := l.requireStatus(req.Id, model.GiftPackStatusPublished); err != nil {
		return nil, err
	}
	if err := l.svcCtx.GiftPackDB.SetStatus(req.Id, model.GiftPackStatusDisabled); err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "停用礼包失败")
	}
	l.packAudit(req.Id, userID, role, "gift_pack_disable", ip, nil)
	got, _ := l.svcCtx.GiftPackDB.Get(req.Id)
	return &types.GiftPackResponse{GiftPack: toPackView(got)}, nil
}

func (l *GiftPackLogic) Revise(req *types.GiftPackPathRequest, authHeader string) (*types.GiftPackResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	p, err := l.svcCtx.GiftPackDB.Get(req.Id)
	if errors.Is(err, model.ErrGiftPackNotFound) {
		return nil, newAPIError(http.StatusNotFound, "礼包不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询礼包失败")
	}
	if p.Status == model.GiftPackStatusDraft {
		return nil, newAPIError(http.StatusConflict, "已是草稿")
	}
	if err := l.svcCtx.GiftPackDB.SetStatus(req.Id, model.GiftPackStatusDraft); err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "回草稿失败")
	}
	l.packAudit(req.Id, userID, role, "gift_pack_revise", ip, map[string]any{"from": p.Status})
	got, _ := l.svcCtx.GiftPackDB.Get(req.Id)
	return &types.GiftPackResponse{GiftPack: toPackView(got)}, nil
}

func (l *GiftPackLogic) List(req *types.GiftPackListRequest, authHeader string) (*types.GiftPackListResponse, error) {
	if _, err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.GiftPackDB.List(req.GameId, req.Status, 100)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询礼包失败")
	}
	items := make([]types.GiftPackView, 0, len(rows))
	for _, r := range rows {
		items = append(items, toPackView(r))
	}
	return &types.GiftPackListResponse{Items: items}, nil
}

func (l *GiftPackLogic) Get(req *types.GiftPackPathRequest, authHeader string) (*types.GiftPackResponse, error) {
	if _, err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	p, err := l.svcCtx.GiftPackDB.Get(req.Id)
	if errors.Is(err, model.ErrGiftPackNotFound) {
		return nil, newAPIError(http.StatusNotFound, "礼包不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询礼包失败")
	}
	return &types.GiftPackResponse{GiftPack: toPackView(p)}, nil
}

// ImportCodes 批量导入码库存（管理面；草稿/已发布均可补码，停用不可）。
func (l *GiftPackLogic) ImportCodes(req *types.ImportCodesRequest, authHeader string) (*types.ImportCodesResponse, error) {
	userID, ip := actor(l.ctx)
	role, err := requireAdminRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	p, err := l.svcCtx.GiftPackDB.Get(req.Id)
	if errors.Is(err, model.ErrGiftPackNotFound) {
		return nil, newAPIError(http.StatusNotFound, "礼包不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询礼包失败")
	}
	if p.Status == model.GiftPackStatusDisabled {
		return nil, newAPIError(http.StatusConflict, "停用礼包不可导入码")
	}
	if len(req.Codes) == 0 {
		return nil, newAPIError(http.StatusBadRequest, "码列表不能为空")
	}
	imported, dup, err := l.svcCtx.GiftPackDB.ImportCodes(req.Id, req.Codes)
	if err != nil {
		l.Errorf("import codes pack=%d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "导入码失败")
	}
	l.packAudit(req.Id, userID, role, "gift_pack_import_codes", ip,
		map[string]any{"imported": imported, "duplicate": dup})
	return &types.ImportCodesResponse{Imported: imported, Duplicate: dup}, nil
}

func (l *GiftPackLogic) ListCodes(req *types.CodeListRequest, authHeader string) (*types.CodeListResponse, error) {
	if _, err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	if _, err := l.svcCtx.GiftPackDB.Get(req.Id); errors.Is(err, model.ErrGiftPackNotFound) {
		return nil, newAPIError(http.StatusNotFound, "礼包不存在")
	}
	rows, err := l.svcCtx.GiftPackDB.ListCodes(req.Id, req.Status, 500)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询码失败")
	}
	codes := make([]types.CodeView, 0, len(rows))
	for _, c := range rows {
		codes = append(codes, toCodeView(c))
	}
	return &types.CodeListResponse{Codes: codes}, nil
}

func (l *GiftPackLogic) ListClaims(req *types.ClaimListAdminRequest, authHeader string) (*types.ClaimListResponse, error) {
	if _, err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.GiftPackDB.ListClaims(req.PackId, req.AccountId, 100)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询领取记录失败")
	}
	claims := make([]types.ClaimView, 0, len(rows))
	for _, c := range rows {
		claims = append(claims, toClaimView(c))
	}
	return &types.ClaimListResponse{Claims: claims}, nil
}

// PlayerList 玩家侧礼包展示：已发布 + 起止覆盖当前 + （可选）区服命中。
func (l *GiftPackLogic) PlayerList(gameID string, serverID int64, authHeader string) (*types.PlayerGiftPackListResponse, error) {
	if _, err := l.requirePlayer(authHeader); err != nil {
		return nil, err
	}
	now := l.now()
	rows, err := l.svcCtx.GiftPackDB.ListPublishedByGame(gameID, now)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询礼包失败")
	}
	accountID := l.accountID()
	items := make([]types.PlayerGiftPackView, 0, len(rows))
	for _, p := range rows {
		if !packTargetHit(p, serverID) {
			continue
		}
		view := types.PlayerGiftPackView{GiftPackView: toPackView(p)}
		view.AvailableCodes, _ = l.svcCtx.GiftPackDB.CountAvailableCodes(p.Id)
		view.ClaimedCount, _ = l.svcCtx.GiftPackDB.CountClaims(p.Id, accountID)
		items = append(items, view)
	}
	return &types.PlayerGiftPackListResponse{Items: items}, nil
}

func (l *GiftPackLogic) PlayerGet(id int64, authHeader string) (*types.PlayerGiftPackView, error) {
	if _, err := l.requirePlayer(authHeader); err != nil {
		return nil, err
	}
	p, err := l.svcCtx.GiftPackDB.Get(id)
	if errors.Is(err, model.ErrGiftPackNotFound) {
		return nil, newAPIError(http.StatusNotFound, "礼包不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询礼包失败")
	}
	if p.Status != model.GiftPackStatusPublished {
		return nil, newAPIError(http.StatusNotFound, "礼包未上架")
	}
	view := types.PlayerGiftPackView{GiftPackView: toPackView(p)}
	view.AvailableCodes, _ = l.svcCtx.GiftPackDB.CountAvailableCodes(p.Id)
	view.ClaimedCount, _ = l.svcCtx.GiftPackDB.CountClaims(p.Id, l.accountID())
	return &view, nil
}

// Claim 玩家领取：限领（per_user_limit）+ 幂等（request_id）+ 码包原子签发。
func (l *GiftPackLogic) Claim(req *types.ClaimGiftPackRequest, authHeader string) (*types.ClaimGiftPackResponse, error) {
	accountID, err := l.requirePlayer(authHeader)
	if err != nil {
		return nil, err
	}
	req.RequestId = strings.TrimSpace(req.RequestId)
	if req.RequestId == "" {
		return nil, newAPIError(http.StatusBadRequest, "request_id 幂等键不能为空")
	}
	p, err := l.svcCtx.GiftPackDB.Get(req.Id)
	if errors.Is(err, model.ErrGiftPackNotFound) {
		return nil, newAPIError(http.StatusNotFound, "礼包不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询礼包失败")
	}
	if p.Status != model.GiftPackStatusPublished {
		return nil, newAPIError(http.StatusConflict, "礼包未上架或已下架")
	}
	now := l.now()
	if now < p.StartAt || now > p.EndAt {
		return nil, newAPIError(http.StatusConflict, "礼包不在活动期内")
	}
	if !packTargetHit(p, req.ServerId) {
		return nil, newAPIError(http.StatusForbidden, "区服不在礼包范围内")
	}
	if err := packAudienceHit(p, req.AudienceType); err != nil {
		return nil, err
	}

	// 幂等：同请求键重提交返回既有领取（不新增、不重复发码）
	if existing, err := l.svcCtx.GiftPackDB.GetClaimByRequest(p.Id, accountID, req.RequestId); err == nil {
		resp := &types.ClaimGiftPackResponse{Claim: toClaimView(existing)}
		if existing.CodeId > 0 {
			if c, err := l.svcCtx.GiftPackDB.GetCode(existing.CodeId); err == nil {
				resp.Code = &types.CodeView{}
				*resp.Code = toCodeView(c)
			}
		}
		return resp, nil
	} else if !errors.Is(err, model.ErrGiftPackClaimNotFound) {
		return nil, newAPIError(http.StatusInternalServerError, "查询领取记录失败")
	}

	count, err := l.svcCtx.GiftPackDB.CountClaims(p.Id, accountID)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "限领查询失败")
	}
	if count >= p.PerUserLimit {
		return nil, newAPIError(http.StatusConflict, "已达限领次数")
	}

	claim := &model.GiftPackClaim{
		PackId: p.Id, AccountId: accountID, GameId: p.GameId,
		ServerId: req.ServerId, RoleId: req.RoleId,
		ClaimIndex: count + 1, RequestId: req.RequestId, Status: model.ClaimStatusClaimed,
	}
	if err := l.svcCtx.GiftPackDB.InsertClaim(claim); err != nil {
		// 唯一冲突：请求键幂等 / 槽位限额并发
		if existing, e2 := l.svcCtx.GiftPackDB.GetClaimByRequest(p.Id, accountID, req.RequestId); e2 == nil {
			resp := &types.ClaimGiftPackResponse{Claim: toClaimView(existing)}
			if existing.CodeId > 0 {
				if c, e3 := l.svcCtx.GiftPackDB.GetCode(existing.CodeId); e3 == nil {
					resp.Code = &types.CodeView{}
					*resp.Code = toCodeView(c)
				}
			}
			return resp, nil
		}
		return nil, newAPIError(http.StatusConflict, "已达限领次数")
	}

	// 码包：原子签发一个可用码（并发安全；无码即释放领取记录并报领完）
	var code *model.GiftPackCode
	if p.CodeRequired {
		codeID, codeStr, err := l.svcCtx.GiftPackDB.IssueCode(p.Id, accountID, claim.Id, 0)
		if err != nil {
			if errors.Is(err, model.ErrGiftPackCodeNotFound) {
				_ = l.svcCtx.GiftPackDB.ReleaseClaim(claim.Id)
				return nil, newAPIError(http.StatusConflict, "礼包码已领完")
			}
			_ = l.svcCtx.GiftPackDB.ReleaseClaim(claim.Id)
			return nil, newAPIError(http.StatusInternalServerError, "签发礼包码失败")
		}
		if err := l.svcCtx.GiftPackDB.BindClaimCode(claim.Id, codeID); err != nil {
			l.Errorf("bind claim code claim=%d code=%d failed: %v", claim.Id, codeID, err)
		}
		// 回读签发后的码（含绑定账号/领取/签发时间），避免手工构造缺字段
		if got, err := l.svcCtx.GiftPackDB.GetCode(codeID); err == nil {
			code = got
		} else {
			code = &model.GiftPackCode{Id: codeID, PackId: p.Id, Code: codeStr,
				Status: model.CodeStatusIssued, AccountId: accountID, ClaimId: claim.Id}
		}
	}

	l.packAudit(p.Id, l.userID(), "player", "gift_pack_claim", "",
		map[string]any{"account_id": accountID, "claim_index": claim.ClaimIndex, "code_id": claim.CodeId})
	resp := &types.ClaimGiftPackResponse{Claim: toClaimView(claim)}
	if code != nil {
		resp.Code = &types.CodeView{}
		*resp.Code = toCodeView(code)
	}
	return resp, nil
}

// MyClaims 玩家领取记录。
func (l *GiftPackLogic) MyClaims(authHeader string) (*types.ClaimListResponse, error) {
	accountID, err := l.requirePlayer(authHeader)
	if err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.GiftPackDB.ListClaimsByAccount(accountID, 100)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询领取记录失败")
	}
	claims := make([]types.ClaimView, 0, len(rows))
	for _, c := range rows {
		claims = append(claims, toClaimView(c))
	}
	return &types.ClaimListResponse{Claims: claims}, nil
}

// Redeem 游戏服核销回调（S2S 签名）：issued → used。实际内容发放归游戏服/
// Courier cdk；本接口只记核销事实（幂等：已 used 视为成功）。验签先于解析；
// 未配置密钥 fail-closed（与 GP-302 评估入口、payments 发货接缝同款）。
func (l *GiftPackLogic) Redeem(rawBody []byte, signature string) error {
	secret := l.svcCtx.Config.Reward.Secret
	if strings.TrimSpace(secret) == "" {
		return newAPIError(http.StatusForbidden, "核销未配置密钥（fail-closed）")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(strings.ToLower(strings.TrimSpace(signature)))) {
		return newAPIError(http.StatusForbidden, "核销签名无效")
	}
	var req types.RedeemGiftPackRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		return newAPIError(http.StatusBadRequest, "核销请求解析失败")
	}
	if req.CodeId <= 0 {
		return newAPIError(http.StatusBadRequest, "code_id 不能为空")
	}
	if _, err := l.svcCtx.GiftPackDB.GetCode(req.CodeId); errors.Is(err, model.ErrGiftPackCodeNotFound) {
		return newAPIError(http.StatusNotFound, "核销码不存在")
	}
	if err := l.svcCtx.GiftPackDB.MarkCodeUsed(req.CodeId); err != nil {
		if errors.Is(err, model.ErrGiftPackCodeNotFound) {
			return nil // 已核销：幂等成功
		}
		return newAPIError(http.StatusInternalServerError, "核销记录失败")
	}
	l.packAudit(req.PackId, 0, "internal", "gift_pack_redeem", "", map[string]any{"code_id": req.CodeId})
	return nil
}

// ---- helpers ----

func (l *GiftPackLogic) requireStatus(id int64, want string) error {
	p, err := l.svcCtx.GiftPackDB.Get(id)
	if errors.Is(err, model.ErrGiftPackNotFound) {
		return newAPIError(http.StatusNotFound, "礼包不存在")
	}
	if err != nil {
		return newAPIError(http.StatusInternalServerError, "查询礼包失败")
	}
	if p.Status != want {
		if want == model.GiftPackStatusDraft {
			return newAPIError(http.StatusConflict, "仅草稿可发布（已发布请先 disable 或 revise）")
		}
		return newAPIError(http.StatusConflict, "仅已发布礼包可停用")
	}
	return nil
}

func (l *GiftPackLogic) ensureStock(id int64) error {
	p, err := l.svcCtx.GiftPackDB.Get(id)
	if err != nil {
		return newAPIError(http.StatusInternalServerError, "查询礼包失败")
	}
	if p.CodeRequired {
		n, err := l.svcCtx.GiftPackDB.CountAvailableCodes(id)
		if err != nil {
			return newAPIError(http.StatusInternalServerError, "查询码库存失败")
		}
		if n == 0 {
			return newAPIError(http.StatusBadRequest, "码包发布前须先导入可用码")
		}
	}
	return nil
}

func (l *GiftPackLogic) requirePlayer(authHeader string) (string, error) {
	if authHeader == "" {
		// 玩家侧路由已过 AuthMiddleware；此处兜底
		return "", newAPIError(http.StatusUnauthorized, "缺少认证")
	}
	return l.accountID(), nil
}

func (l *GiftPackLogic) userID() int64 {
	v, _ := l.ctx.Value("user_id").(int64)
	return v
}

func (l *GiftPackLogic) accountID() string {
	return strconv.FormatInt(l.userID(), 10)
}

func (l *GiftPackLogic) now() string {
	return nowStr()
}

// nowStr 当前 UTC 时间（RFC3339，与 model 表内时间串同口径）。
func nowStr() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (l *GiftPackLogic) packAudit(packID, actorID int64, role, action, ip string, detail any) {
	var detailJSON string
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON = string(b)
		}
	}
	_ = l.svcCtx.ActivityDB.LogAudit(&model.ActivityAuditLog{
		ActivityId: packID, ActorId: actorID, ActorRole: role,
		Action: action, Detail: detailJSON, Ip: ip,
	})
}

// packTargetHit 区服命中（server_ids 空 = 全部）。
func packTargetHit(p *model.GiftPack, serverID int64) bool {
	if strings.TrimSpace(p.ServerIds) == "" {
		return true
	}
	var ids []int64
	if err := json.Unmarshal([]byte(p.ServerIds), &ids); err != nil {
		return false
	}
	if len(ids) == 0 {
		return true
	}
	for _, id := range ids {
		if id == serverID {
			return true
		}
	}
	return false
}

// packAudienceHit 人群命中（audience.type 空或 all 放行；定向要求请求携带
// 相等人群标签，缺省不命中 fail-closed）。
func packAudienceHit(p *model.GiftPack, playerSegment string) error {
	var aud map[string]any
	if err := json.Unmarshal([]byte(p.Audience), &aud); err != nil || len(aud) == 0 {
		return nil
	}
	t, _ := aud["type"].(string)
	if t == "" || t == "all" {
		return nil
	}
	if t != strings.TrimSpace(playerSegment) {
		return newAPIError(http.StatusForbidden, "人群不匹配")
	}
	return nil
}

func toPackView(p *model.GiftPack) types.GiftPackView {
	return types.GiftPackView{
		Id: p.Id, Name: p.Name, GameId: p.GameId, Description: p.Description,
		Status: p.Status, Timezone: p.Timezone, StartAt: p.StartAt, EndAt: p.EndAt,
		ServerIds: unmarshalInts(p.ServerIds), Channel: p.Channel,
		Audience: unmarshalJSON(p.Audience), PerUserLimit: p.PerUserLimit,
		CodeRequired: p.CodeRequired, Content: unmarshalJSON(p.Content),
		CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func toCodeView(c *model.GiftPackCode) types.CodeView {
	return types.CodeView{
		Id: c.Id, PackId: c.PackId, Code: c.Code, Status: c.Status,
		AccountId: c.AccountId, ClaimId: c.ClaimId, IssuedAt: c.IssuedAt,
		UsedAt: c.UsedAt, CreatedAt: c.CreatedAt,
	}
}

func toClaimView(c *model.GiftPackClaim) types.ClaimView {
	return types.ClaimView{
		Id: c.Id, PackId: c.PackId, AccountId: c.AccountId, GameId: c.GameId,
		ServerId: c.ServerId, RoleId: c.RoleId, ClaimIndex: c.ClaimIndex,
		RequestId: c.RequestId, Status: c.Status, CreatedAt: c.CreatedAt,
	}
}
