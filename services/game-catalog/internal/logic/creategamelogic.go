package logic

import (
	"context"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateGameLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateGameLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateGameLogic {
	return &CreateGameLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateGameLogic) CreateGame(req *types.CreateGameRequest) (*types.GameDetailResponse, error) {
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Description) == "" {
		return nil, newAPIError(http.StatusBadRequest, "标题和描述不能为空")
	}

	if len(req.Genres) == 0 {
		return nil, newAPIError(http.StatusBadRequest, "至少需要一个游戏类型")
	}

	if len(req.Platforms) == 0 {
		return nil, newAPIError(http.StatusBadRequest, "至少需要一个平台信息")
	}

	game := &model.Game{
		Title:            strings.TrimSpace(req.Title),
		Description:      strings.TrimSpace(req.Description),
		Genres:           req.Genres,
		Platforms:        req.Platforms,
		ReleaseDate:      req.ReleaseDate,
		Developer:        req.Developer,
		Publisher:        req.Publisher,
		Tags:             req.Tags,
		Score:            req.Score,
		CoverImage:       req.CoverImage,
		Screenshots:      req.Screenshots,
		VideoUrl:         req.VideoUrl,
		Changelog:        req.Changelog,
		DeveloperWebsite: req.DeveloperWebsite,
		StoreLink:        req.StoreLink,
		DownloadUrl:      req.DownloadUrl,
		PlayUrl:          req.PlayUrl,
		TrendingScore:    60,
	}

	entity, err := l.svcCtx.GameRepository.Create(game)
	if err != nil {
		l.Errorf("create game failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "创建失败")
	}
	// GP-004：创建为运营动作，写审计（失败仅记日志，不阻断返回）。
	l.audit("game_create", "game", entity.Id, "", toJSON(toTypesGame(entity)))

	return &types.GameDetailResponse{
		Game: toTypesGame(entity),
	}, nil
}

// audit 记录操作审计；失败仅记日志（写审计不得拖垮主操作）。
// AdminRepository 为 nil（单元测试未装配）时跳过。
func (l *CreateGameLogic) audit(action, resourceType, resourceID, before, after string) {
	if l.svcCtx.AdminRepository == nil {
		return
	}
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
