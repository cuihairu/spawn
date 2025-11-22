// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package logic

import (
	"context"

	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserInfoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserInfoLogic {
	return &GetUserInfoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserInfoLogic) GetUserInfo(req *types.GetUserInfoRequest) (resp *types.ApiResponse, err error) {
	// 验证用户ID
	if req.Id <= 0 {
		return &types.ApiResponse{
			Code:    400,
			Message: "无效的用户ID",
		}, nil
	}

	// 查询用户信息
	user, err := l.svcCtx.UserModel.FindOne(req.Id)
	if err != nil {
		l.Errorw("查询用户失败", logx.Field("user_id", req.Id), logx.Field("error", err))
		return &types.ApiResponse{
			Code:    404,
			Message: "用户不存在",
		}, nil
	}

	// 处理昵称
	var nickname string
	if user.Nickname.Valid {
		nickname = user.Nickname.String
	} else {
		nickname = user.Username
	}

	userInfo := &types.UserInfoResponse{
		Id:       user.Id,
		Username: user.Username,
		Email:    user.Email,
		Nickname: nickname,
	}

	return &types.ApiResponse{
		Code:    200,
		Message: "获取成功",
		Data:    userInfo,
	}, nil
}
