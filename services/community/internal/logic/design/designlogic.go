package design

// DESIGN-001 设计拆解域用例：公开读（品类聚合/列表/详情/评论），登录写
// （发布/编辑自己的拆解、讨论区发言/删自己的评论），精选运营位仅审核管理员。
// 内容过敏感词表（与帖子同款），品类走白名单校验。

import (
	"context"
	"strings"
	"time"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/moderation"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	designTitleMax   = 64
	designSectionMax = 8000 // 单节上限（纯文本，前端分节渲染）
	designCommentMax = 500
)

type DesignLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDesignLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DesignLogic {
	return &DesignLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// Categories 品类入口（公开读；白名单顺序，含 0 行品类）。
func (l *DesignLogic) Categories() (*types.DesignCategoriesResp, error) {
	rows, err := l.svcCtx.DesignRepo.Categories()
	if err != nil {
		return nil, httperr.Internal("聚合品类失败")
	}
	cats := make([]types.DesignCategoryRow, 0, len(rows))
	for _, r := range rows {
		cats = append(cats, types.DesignCategoryRow{Category: r.Category, Count: r.Count})
	}
	return &types.DesignCategoriesResp{Categories: cats}, nil
}

// List 拆解列表（公开读；category/featured 筛选，分页收口）。列表条目裁掉
// 全文分节字段，只回摘要——列表接口不做全文下发。
func (l *DesignLogic) List(req *types.DesignListReq) (*types.DesignListResp, error) {
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	rows, total, err := l.svcCtx.DesignRepo.List(req.Category, req.Featured, limit, offset)
	if err != nil {
		return nil, httperr.Internal("查询拆解列表失败")
	}
	items := make([]types.DesignBreakdown, 0, len(rows))
	for _, r := range rows {
		items = append(items, types.DesignBreakdown{
			Id: r.Id, GameTitle: r.GameTitle, Category: r.Category, Summary: r.Summary,
			AuthorId: r.AuthorId, AuthorName: r.AuthorName, Featured: r.Featured,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return &types.DesignListResp{Items: items, Total: total}, nil
}

// Get 拆解详情（公开读，全文）。
func (l *DesignLogic) Get(id int64) (*types.DesignDetailResp, error) {
	if id <= 0 {
		return nil, httperr.BadRequest("invalid id")
	}
	b, err := l.svcCtx.DesignRepo.Get(id)
	if err != nil {
		return nil, httperr.NotFound("拆解不存在")
	}
	return &types.DesignDetailResp{Breakdown: toType(b)}, nil
}

// Create 发布拆解（登录即可；标题/品类必填，品类走白名单；过敏感词表）。
func (l *DesignLogic) Create(req *types.DesignCreateReq) (*types.DesignDetailResp, error) {
	userId, username, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if err := validateDesign(req); err != nil {
		return nil, err
	}
	if hit, ok := moderation.FirstHit(l.svcCtx.BlockedWords, designSearchText(req)); ok {
		return nil, httperr.BadRequest("内容包含敏感词：" + hit)
	}
	id, err := l.svcCtx.DesignRepo.Create(&model.DesignBreakdown{
		GameTitle: req.GameTitle, Category: req.Category, Summary: req.Summary,
		CoreLoop: req.CoreLoop, Progression: req.Progression, Economy: req.Economy,
		Combat: req.Combat, GrowthLines: req.GrowthLines, Social: req.Social,
		Monetization: req.Monetization, Fairness: req.Fairness,
		WorthStealing: req.WorthStealing, Pitfalls: req.Pitfalls, Sources: req.Sources,
		AuthorId: userId, AuthorName: username,
	})
	if err != nil {
		return nil, httperr.Internal("发布拆解失败")
	}
	b, err := l.svcCtx.DesignRepo.Get(id)
	if err != nil {
		return nil, httperr.Internal("回读拆解失败")
	}
	return &types.DesignDetailResp{Breakdown: toType(b)}, nil
}

// Update 编辑拆解（仅作者；非作者按 404 不泄露可编辑性）。
func (l *DesignLogic) Update(req *types.DesignUpdateReq) (*types.DesignDetailResp, error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req.Id <= 0 {
		return nil, httperr.BadRequest("invalid id")
	}
	if err := validateDesign(&req.DesignCreateReq); err != nil {
		return nil, err
	}
	if hit, ok := moderation.FirstHit(l.svcCtx.BlockedWords, designSearchText(&req.DesignCreateReq)); ok {
		return nil, httperr.BadRequest("内容包含敏感词：" + hit)
	}
	if err := l.svcCtx.DesignRepo.Update(req.Id, userId, &model.DesignBreakdown{
		GameTitle: req.GameTitle, Category: req.Category, Summary: req.Summary,
		CoreLoop: req.CoreLoop, Progression: req.Progression, Economy: req.Economy,
		Combat: req.Combat, GrowthLines: req.GrowthLines, Social: req.Social,
		Monetization: req.Monetization, Fairness: req.Fairness,
		WorthStealing: req.WorthStealing, Pitfalls: req.Pitfalls, Sources: req.Sources,
	}); err != nil {
		return nil, httperr.NotFound("拆解不存在")
	}
	b, err := l.svcCtx.DesignRepo.Get(req.Id)
	if err != nil {
		return nil, httperr.Internal("回读拆解失败")
	}
	return &types.DesignDetailResp{Breakdown: toType(b)}, nil
}

// SetFeatured 精选运营位（仅审核管理员，幂等）。
func (l *DesignLogic) SetFeatured(req *types.DesignFeatureReq) error {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return err
	}
	if req.Id <= 0 {
		return httperr.BadRequest("invalid id")
	}
	if !l.svcCtx.IsAdmin(userId) {
		return httperr.Forbidden("admin permission required")
	}
	return l.svcCtx.DesignRepo.SetFeatured(req.Id, req.Featured)
}

// AddComment 讨论区发言（登录；过敏感词表）。
func (l *DesignLogic) AddComment(req *types.DesignCommentCreateReq) (*types.DesignComment, error) {
	userId, username, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(req.Content)
	if content == "" || len([]rune(content)) > designCommentMax {
		return nil, httperr.BadRequest("评论 1-500 字")
	}
	if _, err := l.svcCtx.DesignRepo.Get(req.BreakdownId); err != nil {
		return nil, httperr.NotFound("拆解不存在")
	}
	if hit, ok := moderation.FirstHit(l.svcCtx.BlockedWords, content); ok {
		return nil, httperr.BadRequest("内容包含敏感词：" + hit)
	}
	id, err := l.svcCtx.DesignRepo.AddComment(&model.DesignComment{
		BreakdownId: req.BreakdownId, AuthorId: userId, AuthorName: username, Content: content,
	})
	if err != nil {
		return nil, httperr.Internal("发言失败")
	}
	return &types.DesignComment{
		Id: id, BreakdownId: req.BreakdownId, AuthorId: userId,
		AuthorName: username, Content: content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// ListComments 讨论区评论列表（公开读）。
func (l *DesignLogic) ListComments(req *types.DesignCommentsReq) (*types.DesignCommentsResp, error) {
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := l.svcCtx.DesignRepo.ListComments(req.BreakdownId, limit, offset)
	if err != nil {
		return nil, httperr.Internal("查询评论失败")
	}
	out := make([]types.DesignComment, 0, len(rows))
	for _, r := range rows {
		out = append(out, types.DesignComment{
			Id: r.Id, BreakdownId: r.BreakdownId, AuthorId: r.AuthorId,
			AuthorName: r.AuthorName, Content: r.Content, CreatedAt: r.CreatedAt,
		})
	}
	return &types.DesignCommentsResp{Comments: out}, nil
}

// DeleteComment 删除自己的评论（他人按不存在处理）。
func (l *DesignLogic) DeleteComment(req *types.DesignCommentDeleteReq) error {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return err
	}
	if req.CommentId <= 0 {
		return httperr.BadRequest("invalid comment id")
	}
	if err := l.svcCtx.DesignRepo.DeleteComment(req.CommentId, userId); err != nil {
		return httperr.NotFound("评论不存在")
	}
	return nil
}

// validateDesign 创建/更新共用校验：标题与品类必填、品类白名单、单节长度上限。
func validateDesign(req *types.DesignCreateReq) error {
	title := strings.TrimSpace(req.GameTitle)
	if title == "" || len([]rune(title)) > designTitleMax {
		return httperr.BadRequest("游戏标题 1-64 字")
	}
	req.GameTitle = title
	if !model.ValidDesignCategory(req.Category) {
		return httperr.BadRequest("品类必须是 " + strings.Join(model.DesignCategories, "/"))
	}
	for _, sec := range []string{req.Summary, req.CoreLoop, req.Progression, req.Economy,
		req.Combat, req.GrowthLines, req.Social, req.Monetization, req.Fairness,
		req.WorthStealing, req.Pitfalls, req.Sources} {
		if len([]rune(sec)) > designSectionMax {
			return httperr.BadRequest("单节内容过长")
		}
	}
	return nil
}

// designSearchText 拼接全部文本节供敏感词扫描。
func designSearchText(req *types.DesignCreateReq) string {
	return req.GameTitle + " " + req.Summary + " " + req.CoreLoop + " " + req.Progression +
		" " + req.Economy + " " + req.Combat + " " + req.GrowthLines + " " + req.Social +
		" " + req.Monetization + " " + req.Fairness + " " + req.WorthStealing +
		" " + req.Pitfalls + " " + req.Sources
}

func toType(b *model.DesignBreakdown) types.DesignBreakdown {
	return types.DesignBreakdown{
		Id: b.Id, GameTitle: b.GameTitle, Category: b.Category, Summary: b.Summary,
		CoreLoop: b.CoreLoop, Progression: b.Progression, Economy: b.Economy,
		Combat: b.Combat, GrowthLines: b.GrowthLines, Social: b.Social,
		Monetization: b.Monetization, Fairness: b.Fairness, WorthStealing: b.WorthStealing,
		Pitfalls: b.Pitfalls, Sources: b.Sources, AuthorId: b.AuthorId,
		AuthorName: b.AuthorName, Featured: b.Featured,
		CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
	}
}
