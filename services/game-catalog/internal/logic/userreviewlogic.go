package logic

// GP-401 游戏评价（F04）：玩家评分（1-5 星覆盖式）、长评（一游戏一评）、
// 评价举报（一人一评一报）、开发者回复（合作方自有游戏，官方单条）。
// 公开读仅对已上架游戏开放（与详情同口径，不泄露未上架游戏的存在性）；
// 管理处置走举报队列（dismiss 驳回 / remove 移除评价）。

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/game-catalog/internal/notify"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// 评价域长度口径：长评 10-4000 字（与列宽一致），举报理由 1-500 字，
// 开发者回复 1-2000 字。
const (
	reviewContentMinRunes = 10
	reviewContentMaxRunes = 4000
	reportReasonMaxRunes  = 500
	devReplyMaxRunes      = 2000
)

type UserReviewLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUserReviewLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserReviewLogic {
	return &UserReviewLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UserReviewLogic) userID() int64 {
	id, _ := l.ctx.Value("user_id").(int64)
	return id
}

// visibleGame 公开评价域入口的可见性口径：游戏存在且已上架，否则 404
// （不区分「不存在」与「未上架」，不泄露存在性）。
func (l *UserReviewLogic) visibleGame(gameID string) error {
	g, err := l.svcCtx.GameRepository.Get(gameID)
	if err != nil {
		if errors.Is(err, model.ErrGameNotFound) {
			return newAPIError(http.StatusNotFound, "游戏不存在")
		}
		l.Errorf("query game failed: %v", err)
		return newAPIError(http.StatusInternalServerError, "查询游戏失败")
	}
	if g.Status != model.GameStatusApproved {
		return newAPIError(http.StatusNotFound, "游戏不存在")
	}
	return nil
}

// Rate 玩家评分（覆盖式；返回写入值与最新聚合）。
func (l *UserReviewLogic) Rate(req *types.RateGameRequest) (*types.RateGameResponse, error) {
	userID := l.userID()
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	if req.Rating < 1 || req.Rating > 5 {
		return nil, newAPIError(http.StatusBadRequest, "评分须为 1-5 星")
	}
	if err := l.visibleGame(req.Id); err != nil {
		return nil, err
	}
	if err := l.svcCtx.AdminRepository.UpsertUserRating(req.Id, userID, req.Rating); err != nil {
		l.Errorf("upsert rating game %s user %d failed: %v", req.Id, userID, err)
		return nil, newAPIError(http.StatusInternalServerError, "写入评分失败")
	}
	summary, err := l.svcCtx.AdminRepository.GetRatingSummary(req.Id)
	if err != nil {
		l.Errorf("query rating summary failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询评分失败")
	}
	return &types.RateGameResponse{
		GameId:  req.Id,
		Rating:  req.Rating,
		Summary: types.RatingSummaryView{Average: summary.Average, Count: summary.Count},
	}, nil
}

// validateReviewContent 长评正文长度口径（首尾空白裁剪后按 rune 计）。
func validateReviewContent(content string) (string, error) {
	c := strings.TrimSpace(content)
	n := len([]rune(c))
	if n < reviewContentMinRunes {
		return "", newAPIError(http.StatusBadRequest, "评价内容不能少于 10 字")
	}
	if n > reviewContentMaxRunes {
		return "", newAPIError(http.StatusBadRequest, "评价内容不能超过 4000 字")
	}
	return c, nil
}

// CreateReview 发长评（一游戏一评，重复 409；已有评改走 PUT）。
func (l *UserReviewLogic) CreateReview(req *types.CreateGameReviewRequest) (*types.UserReviewView, error) {
	userID := l.userID()
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	content, err := validateReviewContent(req.Content)
	if err != nil {
		return nil, err
	}
	if err := l.visibleGame(req.Id); err != nil {
		return nil, err
	}
	r, err := l.svcCtx.AdminRepository.CreateUserReview(&model.UserReview{
		GameId: req.Id, UserId: userID, Content: content,
	})
	if err != nil {
		if errors.Is(err, model.ErrUserReviewExists) {
			return nil, newAPIError(http.StatusConflict, "已评价过该游戏，请改用编辑")
		}
		l.Errorf("create review game %s user %d failed: %v", req.Id, userID, err)
		return nil, newAPIError(http.StatusInternalServerError, "发布评价失败")
	}
	return toTypesUserReview(r), nil
}

// UpdateReview 作者改评（非本人按不存在处理，不泄露他人评价的可编辑性）。
func (l *UserReviewLogic) UpdateReview(req *types.UpdateGameReviewRequest) (*types.UserReviewView, error) {
	userID := l.userID()
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	content, err := validateReviewContent(req.Content)
	if err != nil {
		return nil, err
	}
	current, err := l.svcCtx.AdminRepository.GetUserReview(req.ReviewId)
	if err != nil {
		if errors.Is(err, model.ErrUserReviewNotFound) {
			return nil, newAPIError(http.StatusNotFound, "评价不存在")
		}
		l.Errorf("get review %d failed: %v", req.ReviewId, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询评价失败")
	}
	if current.GameId != req.Id || current.UserId != userID || current.DeletedAt != "" {
		return nil, newAPIError(http.StatusNotFound, "评价不存在")
	}
	updated, err := l.svcCtx.AdminRepository.UpdateUserReview(req.ReviewId, content)
	if err != nil {
		l.Errorf("update review %d failed: %v", req.ReviewId, err)
		return nil, newAPIError(http.StatusInternalServerError, "更新评价失败")
	}
	// 单查无评分 JOIN：回读作者当前评分补齐视图
	if rating, rerr := l.svcCtx.AdminRepository.GetUserRating(req.Id, userID); rerr == nil {
		updated.Rating = rating
	}
	return toTypesUserReview(updated), nil
}

// ListReviews 公开长评列表（新在前，带作者评分与开发者回复）。
func (l *UserReviewLogic) ListReviews(req *types.GameReviewListRequest) (*types.GameReviewListResponse, error) {
	if err := l.visibleGame(req.Id); err != nil {
		return nil, err
	}
	limit, offset := clampPage(req.Limit, req.Offset, 100)
	items, total, err := l.svcCtx.AdminRepository.ListUserReviewsByGame(req.Id, limit, offset, req.Sort == "votes")
	if err != nil {
		l.Errorf("list reviews game %s failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询评价失败")
	}
	out := make([]types.UserReviewView, 0, len(items))
	for _, r := range items {
		out = append(out, *toTypesUserReview(r))
	}
	return &types.GameReviewListResponse{Items: out, Total: total, Limit: int64(limit), Offset: int64(offset)}, nil
}

// VoteReview 长评有用票（一人一评一票，幂等；重复投不重复计数）。评价须
// 属该游戏且未被移除，否则 404。
func (l *UserReviewLogic) VoteReview(req *types.VoteGameReviewRequest) (*types.VoteGameReviewResponse, error) {
	userID := l.userID()
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	if err := l.visibleGame(req.Id); err != nil {
		return nil, err
	}
	votes, err := l.svcCtx.AdminRepository.VoteReview(req.Id, req.ReviewId, userID)
	if errors.Is(err, model.ErrUserReviewNotFound) {
		return nil, newAPIError(http.StatusNotFound, "评价不存在")
	}
	if err != nil {
		l.Errorf("vote review %d user %d failed: %v", req.ReviewId, userID, err)
		return nil, newAPIError(http.StatusInternalServerError, "投票失败")
	}
	return &types.VoteGameReviewResponse{ReviewId: req.ReviewId, Votes: votes, Voted: true}, nil
}

// UnvoteReview 取消长评有用票（幂等：未投过也返回最新票数）。
func (l *UserReviewLogic) UnvoteReview(req *types.VoteGameReviewRequest) (*types.VoteGameReviewResponse, error) {
	userID := l.userID()
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	if err := l.visibleGame(req.Id); err != nil {
		return nil, err
	}
	votes, err := l.svcCtx.AdminRepository.UnvoteReview(req.Id, req.ReviewId, userID)
	if errors.Is(err, model.ErrUserReviewNotFound) {
		return nil, newAPIError(http.StatusNotFound, "评价不存在")
	}
	if err != nil {
		l.Errorf("unvote review %d user %d failed: %v", req.ReviewId, userID, err)
		return nil, newAPIError(http.StatusInternalServerError, "取消投票失败")
	}
	return &types.VoteGameReviewResponse{ReviewId: req.ReviewId, Votes: votes, Voted: false}, nil
}

// ReportReview 举报长评（幂等：重复举报返回既有记录 created=false）。
func (l *UserReviewLogic) ReportReview(req *types.ReportGameReviewRequest) (*types.ReportGameReviewResponse, error) {
	userID := l.userID()
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return nil, newAPIError(http.StatusBadRequest, "举报理由不能为空")
	}
	if len([]rune(reason)) > reportReasonMaxRunes {
		return nil, newAPIError(http.StatusBadRequest, "举报理由不能超过 500 字")
	}
	if err := l.visibleGame(req.Id); err != nil {
		return nil, err
	}
	current, err := l.svcCtx.AdminRepository.GetUserReview(req.ReviewId)
	if err != nil || current.GameId != req.Id || current.DeletedAt != "" {
		return nil, newAPIError(http.StatusNotFound, "评价不存在")
	}
	r, created, err := l.svcCtx.AdminRepository.CreateReviewReport(&model.ReviewReport{
		ReviewId: req.ReviewId, GameId: req.Id, ReporterId: userID, Reason: reason,
	})
	if err != nil {
		l.Errorf("create review report review %d user %d failed: %v", req.ReviewId, userID, err)
		return nil, newAPIError(http.StatusInternalServerError, "提交举报失败")
	}
	return &types.ReportGameReviewResponse{Id: r.Id, Created: created}, nil
}

// ReplyReview 开发者回复（合作方仅限自有游戏；官方单条，重复调用覆盖更新）。
func (l *UserReviewLogic) ReplyReview(req *types.ReplyGameReviewRequest) (*types.UserReviewView, error) {
	partnerID, err := l.partnerIDOf()
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, newAPIError(http.StatusBadRequest, "回复内容不能为空")
	}
	if len([]rune(content)) > devReplyMaxRunes {
		return nil, newAPIError(http.StatusBadRequest, "回复内容不能超过 2000 字")
	}
	if err := l.ownGameOf(partnerID, req.Id); err != nil {
		return nil, err
	}
	current, err := l.svcCtx.AdminRepository.GetUserReview(req.ReviewId)
	if err != nil || current.GameId != req.Id || current.DeletedAt != "" {
		return nil, newAPIError(http.StatusNotFound, "评价不存在")
	}
	userID, _ := l.ctx.Value("user_id").(int64)
	updated, err := l.svcCtx.AdminRepository.UpsertDevReply(req.ReviewId, userID, content)
	if err != nil {
		l.Errorf("upsert dev reply review %d failed: %v", req.ReviewId, err)
		return nil, newAPIError(http.StatusInternalServerError, "写入回复失败")
	}
	// 单查无评分 JOIN：回读作者当前评分补齐视图
	if rating, rerr := l.svcCtx.AdminRepository.GetUserRating(req.Id, current.UserId); rerr == nil {
		updated.Rating = rating
	}
	l.audit("review_dev_reply", "game_user_review", formatInt(req.ReviewId), toJSON(toTypesUserReview(current)), toJSON(toTypesUserReview(updated)))
	return toTypesUserReview(updated), nil
}

// partnerIDOf 取当前账号绑定合作方 id；未绑定 403。
func (l *UserReviewLogic) partnerIDOf() (int64, error) {
	userID := l.userID()
	_, partnerID, err := l.svcCtx.AdminRepository.GetRoleWithPartner(userID)
	if err != nil {
		l.Errorf("query role binding failed: %v", err)
		return 0, newAPIError(http.StatusForbidden, "当前账号未绑定合作方")
	}
	if partnerID <= 0 {
		return 0, newAPIError(http.StatusForbidden, "当前账号未绑定合作方")
	}
	return partnerID, nil
}

// ownGameOf 校验游戏归属当前合作方（active）；与 PartnerLogic.ownGame 同款语义。
func (l *UserReviewLogic) ownGameOf(partnerID int64, gameID string) error {
	ownership, err := l.svcCtx.AdminRepository.GetOwnership(gameID)
	if err != nil {
		if errors.Is(err, model.ErrOwnershipMissing) {
			return newAPIError(http.StatusNotFound, "游戏不存在")
		}
		l.Errorf("query game ownership failed: %v", err)
		return newAPIError(http.StatusInternalServerError, "查询游戏归属失败")
	}
	if ownership.PartnerId != partnerID || ownership.Status != model.OwnershipActive {
		return newAPIError(http.StatusForbidden, "该游戏不属于当前合作方")
	}
	return nil
}

// audit 记录操作审计（合作方/管理动作；失败仅记日志）。
func (l *UserReviewLogic) audit(action, resourceType, resourceID, before, after string) {
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

func formatInt(v int64) string {
	return strconvFormatInt(v)
}

// ---- 管理面：举报队列处置 ----

// ListReviewReports 管理举报队列（status 空为全部）。
func (l *UserReviewLogic) ListReviewReports(req *types.ListReviewReportsRequest) (*types.ReviewReportsListResponse, error) {
	limit, offset := clampPage(req.Limit, req.Offset, 100)
	items, total, err := l.svcCtx.AdminRepository.ListReviewReports(strings.TrimSpace(req.Status), limit, offset)
	if err != nil {
		l.Errorf("list review reports failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询举报队列失败")
	}
	out := make([]types.ReviewReportView, 0, len(items))
	for _, r := range items {
		out = append(out, *toTypesReviewReport(r))
	}
	return &types.ReviewReportsListResponse{Items: out, Total: total}, nil
}

// HandleReviewReport 处置举报：dismiss 驳回（评价保留）；remove 移除评价
// （同评其余未决举报一并结案，队列不留悬空未决项）。未决才可处置，已处置 409。
func (l *UserReviewLogic) HandleReviewReport(req *types.HandleReviewReportRequest) (*types.ReviewReportView, error) {
	var status string
	var action string
	switch req.Action {
	case "dismiss":
		status, action = model.ReviewReportDismissed, "review_report_dismiss"
	case "remove":
		status, action = model.ReviewReportResolved, "review_report_remove"
	default:
		return nil, newAPIError(http.StatusBadRequest, "处置动作须为 dismiss/remove")
	}
	report, err := l.svcCtx.AdminRepository.GetReviewReport(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrReviewReportNotFound) {
			return nil, newAPIError(http.StatusNotFound, "举报不存在")
		}
		l.Errorf("get review report %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询举报失败")
	}
	actorID, _ := l.ctx.Value("user_id").(int64)
	ok, err := l.svcCtx.AdminRepository.DecideReviewReportIfPending(req.Id, actorID, status)
	if err != nil {
		l.Errorf("decide review report %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "处置举报失败")
	}
	if !ok {
		return nil, newAPIError(http.StatusConflict, "该举报已处置")
	}
	// GP-401 处置结果通知受众：remove 需在删除/结案前捕获评价作者与同评其余
	// 未决举报人（结案后不再可查），二者均取不到时静默降级（无通知不阻断）。
	var authorID int64
	var siblingReporters []int64
	if req.Action == "remove" {
		if review, err := l.svcCtx.AdminRepository.GetUserReview(report.ReviewId); err == nil {
			authorID = review.UserId
		} else if !errors.Is(err, model.ErrUserReviewNotFound) {
			l.Errorf("get review %d for notify failed: %v", report.ReviewId, err)
		}
		if ids, err := l.svcCtx.AdminRepository.ListPendingReportersByReview(report.ReviewId); err == nil {
			siblingReporters = ids
		} else {
			l.Errorf("list pending reporters review %d failed: %v", report.ReviewId, err)
		}
	}
	if req.Action == "remove" {
		if err := l.svcCtx.AdminRepository.DeleteUserReview(report.ReviewId); err != nil {
			l.Errorf("delete review %d failed: %v", report.ReviewId, err)
			return nil, newAPIError(http.StatusInternalServerError, "移除评价失败")
		}
		if err := l.svcCtx.AdminRepository.ResolvePendingReportsForReview(report.ReviewId, actorID); err != nil {
			l.Errorf("resolve sibling reports review %d failed: %v", report.ReviewId, err)
		}
	}
	final, err := l.svcCtx.AdminRepository.GetReviewReport(req.Id)
	if err != nil {
		l.Errorf("reload review report %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询举报失败")
	}
	l.audit(action, "review_report", formatInt(req.Id), toJSON(toTypesReviewReport(report)), toJSON(toTypesReviewReport(final)))
	// GP-401 举报处置通知回路：dismiss 告知举报人评价保留；remove 告知各举报人
	// （含同评自动结案的）评价已移除、并告知评价作者。fire-and-forget，失败仅记
	// 日志，绝不回改处置结果（与 game_launch/game_version 同款契约）。
	if req.Action == "dismiss" {
		l.notifyReviewReportResult(report.ReviewId, distinctPositive([]int64{report.ReporterId}),
			"您的举报已复核：该评价未违反社区规范，评价保留")
	} else {
		reporters := append([]int64{report.ReporterId}, siblingReporters...)
		l.notifyReviewReportResult(report.ReviewId, distinctPositive(reporters),
			"您的举报已复核：该评价已被移除")
		l.notifyReviewRemoved(report.ReviewId, authorID)
	}
	return toTypesReviewReport(final), nil
}

// RestoreReview 恢复被移除的评价（管理面，platform_admin/operator）：清软删
// 标记回公开列表，内容与开发者回复原样保留；已恢复/不存在 → 409/404。
func (l *UserReviewLogic) RestoreReview(req *types.RestoreReviewRequest) (*types.UserReviewView, error) {
	r, err := l.svcCtx.AdminRepository.GetUserReview(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrUserReviewNotFound) {
			return nil, newAPIError(http.StatusNotFound, "评价不存在")
		}
		l.Errorf("get review %d for restore failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询评价失败")
	}
	if r.DeletedAt == "" {
		return nil, newAPIError(http.StatusConflict, "评价未被移除，无需恢复")
	}
	ok, err := l.svcCtx.AdminRepository.RestoreUserReview(req.Id)
	if err != nil {
		l.Errorf("restore review %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "恢复评价失败")
	}
	if !ok {
		// 并发：已被他人恢复或行已不存在
		return nil, newAPIError(http.StatusConflict, "评价已恢复或不存在")
	}
	updated, err := l.svcCtx.AdminRepository.GetUserReview(req.Id)
	if err != nil {
		l.Errorf("reload review %d after restore failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "读取评价失败")
	}
	l.audit("review_restore", "game_user_review", formatInt(req.Id), toJSON(toTypesUserReview(r)), toJSON(toTypesUserReview(updated)))
	// GP-401 恢复通知：告知作者评价已恢复（与移除通知对称；未配置 no-op）。
	l.notifyReviewRestored(req.Id, updated.UserId)
	return toTypesUserReview(updated), nil
}

// notifyReviewReportResult 向举报人扇出处置结论（community /internal/notifications，
// 未配置时 no-op 跳过）。
func (l *UserReviewLogic) notifyReviewReportResult(reviewId int64, reporters []int64, content string) {
	if !l.svcCtx.Notify.Enabled() || len(reporters) == 0 {
		return
	}
	l.svcCtx.Notify.Publish(notify.Notification{
		UserIds: reporters, Type: "review_report_result", TargetId: reviewId,
		Content: content,
	})
}

// notifyReviewRemoved 向评价作者扇出评价被移除通知。
func (l *UserReviewLogic) notifyReviewRemoved(reviewId, authorID int64) {
	if !l.svcCtx.Notify.Enabled() || authorID <= 0 {
		return
	}
	l.svcCtx.Notify.Publish(notify.Notification{
		UserIds: []int64{authorID}, Type: "review_removed", TargetId: reviewId,
		Content: "您的评价因收到有效举报，已被移除",
	})
}

// notifyReviewRestored 向评价作者扇出评价被恢复通知（与移除通知对称）。
func (l *UserReviewLogic) notifyReviewRestored(reviewId, authorID int64) {
	if !l.svcCtx.Notify.Enabled() || authorID <= 0 {
		return
	}
	l.svcCtx.Notify.Publish(notify.Notification{
		UserIds: []int64{authorID}, Type: "review_restored", TargetId: reviewId,
		Content: "您被移除的评价已恢复，可在游戏页重新查看",
	})
}

// distinctPositive 去重并剔除非正 user id（通知受众容错：0/负 id 不投递）。
func distinctPositive(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
