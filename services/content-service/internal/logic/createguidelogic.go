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
	// TODO: 从上下文中获取当前用户ID和名称
	// 这里暂时硬编码，实际应该从JWT或session中获取
	authorId := int64(1001)
	authorName := "测试作者"

	guide := &model.Guide{
		GameId:     req.GameId,
		GameTitle:  req.GameId, // TODO: 实际应该查询game-catalog服务获取游戏名称
		Title:      req.Title,
		Content:    req.Content,
		Summary:    req.Summary,
		CoverImage: req.CoverImage,
		AuthorId:   authorId,
		AuthorName: authorName,
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
