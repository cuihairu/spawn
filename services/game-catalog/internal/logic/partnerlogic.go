package logic

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// PartnerLogic 合作方自助逻辑（GP-103 开发者自助入口）：
// 仅可见/可操作归属本人合作方的游戏；资料与版本走同一提审流。
type PartnerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPartnerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PartnerLogic {
	return &PartnerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// partnerID 取当前账号绑定的合作方 id；角色非 partner 或未绑定为 0。
func (l *PartnerLogic) partnerID() int64 {
	userID, _ := l.ctx.Value("user_id").(int64)
	_, partnerID, err := l.svcCtx.AdminRepository.GetRoleWithPartner(userID)
	if err != nil {
		l.Errorf("query role binding failed: %v", err)
		return 0
	}
	return partnerID
}

// ownGame 校验游戏归属当前合作方（active），否则 403/404。
func (l *PartnerLogic) ownGame(partnerID int64, gameID string) error {
	ownership, err := l.svcCtx.AdminRepository.GetOwnership(gameID)
	if err != nil {
		if errors.Is(err, model.ErrOwnershipMissing) {
			return newAPIError(http.StatusNotFound, "游戏不存在")
		}
		return newAPIError(http.StatusInternalServerError, "查询游戏归属失败")
	}
	if ownership.PartnerId != partnerID || ownership.Status != model.OwnershipActive {
		return newAPIError(http.StatusForbidden, "该游戏不属于当前合作方")
	}
	return nil
}

// ListPartnerGames 列出本合作方游戏（含全部状态：提审中/驳回也可见）。
func (l *PartnerLogic) ListPartnerGames() (*types.ListGamesResponse, error) {
	partnerID := l.partnerID()
	if partnerID <= 0 {
		return nil, newAPIError(http.StatusForbidden, "当前账号未绑定合作方")
	}
	gameIDs, err := l.svcCtx.AdminRepository.ListGamesByPartner(partnerID)
	if err != nil {
		l.Errorf("list partner games failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询合作方游戏失败")
	}
	games := make([]types.Game, 0, len(gameIDs))
	for _, id := range gameIDs {
		g, err := l.svcCtx.GameRepository.Get(id)
		if err != nil {
			continue // 归属残留（游戏被物理删除）：跳过不阻断
		}
		games = append(games, toTypesGame(g))
	}
	return &types.ListGamesResponse{Games: games, Total: len(games)}, nil
}

// Submit 资料/版本提审：置 pending 并登记提审记录；同资源存在未决提审时拒绝重复提交。
func (l *PartnerLogic) Submit(req *types.PartnerSubmitRequest) (*types.GameReview, error) {
	partnerID := l.partnerID()
	if partnerID <= 0 {
		return nil, newAPIError(http.StatusForbidden, "当前账号未绑定合作方")
	}
	if strings.TrimSpace(req.Id) == "" {
		return nil, newAPIError(http.StatusBadRequest, "游戏 id 不能为空")
	}
	if err := l.ownGame(partnerID, req.Id); err != nil {
		return nil, err
	}
	userID, _ := l.ctx.Value("user_id").(int64)

	// 版本提审：校验版本存在且属于该游戏
	if req.VersionId != "" {
		v, err := l.svcCtx.AdminRepository.GetVersion(req.VersionId)
		if err != nil {
			if errors.Is(err, model.ErrVersionNotFound) {
				return nil, newAPIError(http.StatusNotFound, "版本不存在")
			}
			return nil, newAPIError(http.StatusInternalServerError, "查询版本失败")
		}
		if v.GameId != req.Id {
			return nil, newAPIError(http.StatusBadRequest, "版本不属于该游戏")
		}
	}

	// 重复提审防护：同资源未决记录存在则拒绝
	pending, total, err := l.svcCtx.AdminRepository.ListReviews(model.ReviewPending, 1000, 0)
	if err != nil {
		l.Errorf("list pending reviews failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询提审队列失败")
	}
	_ = total
	for _, r := range pending {
		if r.GameId == req.Id && r.VersionId == req.VersionId {
			return nil, newAPIError(http.StatusConflict, "该资源已有待审核的提审")
		}
	}

	review, err := l.svcCtx.AdminRepository.SubmitReview(req.Id, req.VersionId, userID)
	if err != nil {
		l.Errorf("submit review failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "登记提审失败")
	}
	if req.VersionId == "" {
		if _, err := l.svcCtx.GameRepository.SetStatus(req.Id, model.GameStatusPending, ""); err != nil {
			l.Errorf("set game pending failed: %v", err)
			return nil, newAPIError(http.StatusInternalServerError, "更新游戏状态失败")
		}
	} else if _, err := l.svcCtx.AdminRepository.SetVersionStatus(req.VersionId, model.VersionStatusPending, ""); err != nil {
		l.Errorf("set version pending failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "更新版本状态失败")
	}

	l.audit("submit_review", "game", req.Id, "", toJSON(toReview(review)))
	return toReview(review), nil
}

// audit 记录操作审计（合作方动作同样入审计）。
func (l *PartnerLogic) audit(action, resourceType, resourceID, before, after string) {
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

// CreateVersion 合作方为自有游戏创建版本（draft，随后提审）。
func (l *PartnerLogic) CreateVersion(req *types.CreateVersionRequest) (*types.GameVersion, error) {
	partnerID := l.partnerID()
	if partnerID <= 0 {
		return nil, newAPIError(http.StatusForbidden, "当前账号未绑定合作方")
	}
	if strings.TrimSpace(req.Version) == "" {
		return nil, newAPIError(http.StatusBadRequest, "版本号不能为空")
	}
	if err := l.ownGame(partnerID, req.Id); err != nil {
		return nil, err
	}
	v, err := l.svcCtx.AdminRepository.CreateVersion(req.Id, req.Version, req.Changelog)
	if err != nil {
		l.Errorf("create version failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "创建版本失败")
	}
	l.audit("version_create", "version", v.Id, "", toJSON(toVersion(v)))
	return toVersion(v), nil
}

// ListVersions 查看自有游戏的版本列表。
func (l *PartnerLogic) ListVersions(gameID string) (*types.VersionListResponse, error) {
	partnerID := l.partnerID()
	if partnerID <= 0 {
		return nil, newAPIError(http.StatusForbidden, "当前账号未绑定合作方")
	}
	if err := l.ownGame(partnerID, gameID); err != nil {
		return nil, err
	}
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
