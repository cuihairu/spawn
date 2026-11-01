package logic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// AdminLogic 运营后台逻辑（GP-103）：角色/合作方/归属/审核/上下架/版本/审计。
// 所有变更操作写审计日志；审计失败仅记日志不阻断主操作。
type AdminLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAdminLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AdminLogic {
	return &AdminLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// audit 记录操作审计；失败仅记日志（写审计不得拖垮主操作）。
func (l *AdminLogic) audit(action, resourceType, resourceID, before, after string) {
	actorID, _ := l.ctx.Value("user_id").(int64)
	role, _ := l.ctx.Value("role").(string)
	ip, _ := l.ctx.Value("client_ip").(string)
	if err := l.svcCtx.AdminRepository.WriteAudit(&model.AuditLog{
		ActorId: actorID, ActorRole: role, Action: action,
		ResourceType: resourceType, ResourceId: resourceID,
		BeforeJson: before, AfterJson: after, Ip: ip,
	}); err != nil {
		l.Errorf("write audit failed: %v", err)
	}
}

func toJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func (l *AdminLogic) GrantRole(req *types.GrantRoleRequest) (*types.GrantRoleResponse, error) {
	if req.UserId <= 0 {
		return nil, newAPIError(http.StatusBadRequest, "用户 id 非法")
	}
	if req.Role == model.RolePartner && req.PartnerId > 0 {
		// partner 角色需绑定已存在的合作方主体
		if _, err := l.svcCtx.AdminRepository.GetPartner(req.PartnerId); err != nil {
			if errors.Is(err, model.ErrPartnerNotFound) {
				return nil, newAPIError(http.StatusNotFound, "合作方不存在")
			}
			return nil, newAPIError(http.StatusInternalServerError, "查询合作方失败")
		}
	}
	if err := l.svcCtx.AdminRepository.SetRole(req.UserId, req.Role, req.PartnerId); err != nil {
		l.Errorf("grant role failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "写入角色失败")
	}
	l.audit("role_grant", "role", "", "", toJSON(req))
	return &types.GrantRoleResponse{UserId: req.UserId, Role: req.Role, PartnerId: req.PartnerId}, nil
}

func (l *AdminLogic) CreatePartner(req *types.CreatePartnerRequest) (*types.Partner, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, newAPIError(http.StatusBadRequest, "合作方名称不能为空")
	}
	p, err := l.svcCtx.AdminRepository.CreatePartner(req.Name, req.ContactEmail)
	if err != nil {
		l.Errorf("create partner failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "创建合作方失败")
	}
	l.audit("partner_create", "partner", "", "", toJSON(toPartner(p)))
	return toPartner(p), nil
}

func (l *AdminLogic) ListPartners() (*types.PartnerListResponse, error) {
	partners, err := l.svcCtx.AdminRepository.ListPartners()
	if err != nil {
		l.Errorf("list partners failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询合作方失败")
	}
	out := make([]types.Partner, 0, len(partners))
	for _, p := range partners {
		out = append(out, *toPartner(p))
	}
	return &types.PartnerListResponse{Partners: out}, nil
}

func (l *AdminLogic) SetPartnerStatus(req *types.SetPartnerStatusRequest) (*types.Partner, error) {
	before, err := l.svcCtx.AdminRepository.GetPartner(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrPartnerNotFound) {
			return nil, newAPIError(http.StatusNotFound, "合作方不存在")
		}
		l.Errorf("get partner failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询合作方失败")
	}
	p, err := l.svcCtx.AdminRepository.SetPartnerStatus(req.Id, req.Status)
	if err != nil {
		l.Errorf("set partner status failed: %v", err)
		return nil, newAPIError(http.StatusBadRequest, err.Error())
	}
	l.audit("partner_status", "partner", "", toJSON(toPartner(before)), toJSON(toPartner(p)))
	return toPartner(p), nil
}

func (l *AdminLogic) AssignOwnership(req *types.AssignOwnershipRequest) (*types.GameOwnership, error) {
	if _, err := l.svcCtx.GameRepository.Get(req.Id); err != nil {
		if errors.Is(err, model.ErrGameNotFound) {
			return nil, newAPIError(http.StatusNotFound, "游戏不存在")
		}
		return nil, newAPIError(http.StatusInternalServerError, "查询游戏失败")
	}
	if _, err := l.svcCtx.AdminRepository.GetPartner(req.PartnerId); err != nil {
		if errors.Is(err, model.ErrPartnerNotFound) {
			return nil, newAPIError(http.StatusNotFound, "合作方不存在")
		}
		return nil, newAPIError(http.StatusInternalServerError, "查询合作方失败")
	}
	actorID, _ := l.ctx.Value("user_id").(int64)
	before, _ := l.svcCtx.AdminRepository.GetOwnership(req.Id)
	o, err := l.svcCtx.AdminRepository.AssignOwnership(req.Id, req.PartnerId, actorID)
	if err != nil {
		l.Errorf("assign ownership failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "写入游戏归属失败")
	}
	l.audit("ownership_assign", "game", req.Id, toJSON(toOwnership(before)), toJSON(toOwnership(o)))
	return toOwnership(o), nil
}

func (l *AdminLogic) ListReviews(req *types.ListReviewsRequest) (*types.ReviewsListResponse, error) {
	limit, offset := clampPage(req.Limit, req.Offset, 100)
	reviews, total, err := l.svcCtx.AdminRepository.ListReviews(strings.TrimSpace(req.Status), limit, offset)
	if err != nil {
		l.Errorf("list reviews failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询审核队列失败")
	}
	out := make([]types.GameReview, 0, len(reviews))
	for _, r := range reviews {
		out = append(out, *toReview(r))
	}
	return &types.ReviewsListResponse{Reviews: out, Total: total}, nil
}

func (l *AdminLogic) DecideReview(req *types.DecideReviewRequest) (*types.GameReview, error) {
	if req.Decision != model.ReviewApprove && req.Decision != model.ReviewReject {
		return nil, newAPIError(http.StatusBadRequest, "审核决定须为 approve/reject")
	}
	if req.Decision == model.ReviewReject && strings.TrimSpace(req.Reason) == "" {
		return nil, newAPIError(http.StatusBadRequest, "驳回必须填写审核原因")
	}
	review, err := l.svcCtx.AdminRepository.GetReview(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrReviewNotFound) {
			return nil, newAPIError(http.StatusNotFound, "提审不存在")
		}
		return nil, newAPIError(http.StatusInternalServerError, "查询提审失败")
	}
	actorID, _ := l.ctx.Value("user_id").(int64)
	review, err = l.svcCtx.AdminRepository.DecideReview(req.Id, actorID, req.Decision, req.Reason)
	if err != nil {
		if strings.Contains(err.Error(), "已审结") {
			return nil, newAPIError(http.StatusConflict, "该提审已审结")
		}
		l.Errorf("decide review failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "更新审核失败")
	}

	// 同步资源状态：资料提审（version_id 空）写游戏状态；版本提审写版本状态。
	var statusErr error
	if review.VersionId == "" {
		if req.Decision == model.ReviewApprove {
			_, statusErr = l.svcCtx.GameRepository.SetStatus(review.GameId, model.GameStatusApproved, "")
		} else {
			_, statusErr = l.svcCtx.GameRepository.SetStatus(review.GameId, model.GameStatusRejected, req.Reason)
		}
	} else {
		if req.Decision == model.ReviewApprove {
			_, statusErr = l.svcCtx.AdminRepository.SetVersionStatus(review.VersionId, model.VersionStatusApproved, "")
		} else {
			_, statusErr = l.svcCtx.AdminRepository.SetVersionStatus(review.VersionId, model.VersionStatusRejected, req.Reason)
		}
	}
	if statusErr != nil {
		l.Errorf("sync resource status after review failed: %v", statusErr)
		return nil, newAPIError(http.StatusInternalServerError, "同步资源状态失败")
	}

	l.audit("review_"+req.Decision, "game", review.GameId, toJSON(toReview(review)), "")
	return toReview(review), nil
}

func (l *AdminLogic) setGameStatus(gameID, status, action, reason string) (*types.Game, error) {
	before, err := l.svcCtx.GameRepository.Get(gameID)
	if err != nil {
		if errors.Is(err, model.ErrGameNotFound) {
			return nil, newAPIError(http.StatusNotFound, "游戏不存在")
		}
		return nil, newAPIError(http.StatusInternalServerError, "查询游戏失败")
	}
	after, err := l.svcCtx.GameRepository.SetStatus(gameID, status, reason)
	if err != nil {
		l.Errorf("%s failed: %v", action, err)
		return nil, newAPIError(http.StatusInternalServerError, "更新游戏状态失败")
	}
	l.audit(action, "game", gameID, toJSON(toTypesGame(before)), toJSON(toTypesGame(after)))
	g := toTypesGame(after)
	return &g, nil
}

func (l *AdminLogic) PublishGame(id string) (*types.Game, error) {
	return l.setGameStatus(id, model.GameStatusApproved, "game_publish", "")
}

func (l *AdminLogic) UnpublishGame(id string) (*types.Game, error) {
	return l.setGameStatus(id, model.GameStatusOffline, "game_unpublish", "")
}

func (l *AdminLogic) ListVersions(gameID string) (*types.VersionListResponse, error) {
	versions, err := l.svcCtx.AdminRepository.ListVersionsByGame(gameID)
	if err != nil {
		l.Errorf("list versions failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询版本失败")
	}
	out := make([]types.GameVersion, 0, len(versions))
	for _, v := range versions {
		out = append(out, *toVersion(v))
	}
	return &types.VersionListResponse{Versions: out}, nil
}

func (l *AdminLogic) ListAuditLogs(req *types.ListAuditRequest) (*types.AuditListResponse, error) {
	limit, offset := clampPage(req.Limit, req.Offset, 200)
	logs, total, err := l.svcCtx.AdminRepository.ListAuditLogs(strings.TrimSpace(req.ResourceType), limit, offset)
	if err != nil {
		l.Errorf("list audit failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询审计失败")
	}
	out := make([]types.AuditLog, 0, len(logs))
	for _, log := range logs {
		out = append(out, *toAudit(log))
	}
	return &types.AuditListResponse{Logs: out, Total: total}, nil
}

// Me 返回当前登录账号的平台角色（导航/入口权限用）。
// 角色优先取中间件写入上下文的值，归属合作方直查（非 partner 角色为 0）。
func (l *AdminLogic) Me() (*types.MeResponse, error) {
	userID, _ := l.ctx.Value("user_id").(int64)
	username, _ := l.ctx.Value("username").(string)
	role, _ := l.ctx.Value("role").(string)
	dbRole, partnerID, err := l.svcCtx.AdminRepository.GetRoleWithPartner(userID)
	if err != nil {
		l.Errorf("get role failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询角色失败")
	}
	if role == "" {
		role = dbRole
	}
	return &types.MeResponse{UserId: userID, Username: username, Role: role, PartnerId: partnerID}, nil
}

func clampPage(limit, offset, maxLimit int64) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return int(limit), int(offset)
}
