package logic

import (
	"context"
	"net/http"

	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateGuideLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateGuideLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateGuideLogic {
	return &CreateGuideLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateGuideLogic) CreateGuide(req *types.CreateGuideRequest) (resp *types.CreateGuideResponse, err error) {
	// 从上下文中获取当前用户ID和名称
	userId, ok := l.ctx.Value("user_id").(int64)
	if !ok || userId == 0 {
		l.Logger.Error("无法从上下文获取用户ID")
		return &types.CreateGuideResponse{
			Code:    http.StatusUnauthorized,
			Message: "用户认证失败",
		}, nil
	}

	username, ok := l.ctx.Value("username").(string)
	if !ok || username == "" {
		username = "未知用户"
	}

	// 从 game-catalog 服务获取游戏名称
	gameTitle := req.GameId // 默认使用 gameId
	gameInfo, err := l.svcCtx.GameCatalogClient.GetGameById(l.ctx, req.GameId)
	if err != nil {
		l.Logger.Errorw("获取游戏信息失败，使用游戏ID作为标题",
			logx.Field("gameId", req.GameId),
			logx.Field("error", err),
		)
	} else {
		gameTitle = gameInfo.Title
	}

	guide := &model.Guide{
		GameId:     req.GameId,
		GameTitle:  gameTitle,
		Title:      req.Title,
		Content:    req.Content,
		Summary:    req.Summary,
		CoverImage: req.CoverImage,
		AuthorId:   userId,
		AuthorName: username,
		Tags:       req.Tags,
	}

	created, err := l.svcCtx.GuideRepository.Create(guide)
	if err != nil {
		l.Logger.Errorf("create guide failed: %v", err)
		return &types.CreateGuideResponse{
			Code:    http.StatusInternalServerError,
			Message: "创建攻略失败",
		}, nil
	}

	return &types.CreateGuideResponse{
		Code:    http.StatusOK,
		Message: "创建成功",
		Data:    modelGuideToType(created),
	}, nil
}
