// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package logic

import (
	"context"

	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/internal/types"
	"github.com/tappi/tappi/services/user-service/model"
	"github.com/tappi/tappi/services/user-service/utils"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserInfoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateUserInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserInfoLogic {
	return &UpdateUserInfoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateUserInfoLogic) UpdateUserInfo(req *types.UpdateUserInfoRequest) (resp *types.ApiResponse, err error) {
	// 验证用户ID
	if req.Id <= 0 {
		return &types.ApiResponse{
			Code:    400,
			Message: "无效的用户ID",
		}, nil
	}

	// 验证邮箱格式
	if req.Email != "" {
		if err := utils.ValidateEmail(req.Email); err != nil {
			return &types.ApiResponse{
				Code:    400,
				Message: err.Error(),
			}, nil
		}
	}

	// 查询用户是否存在
	user, err := l.svcCtx.UserModel.FindOne(req.Id)
	if err != nil {
		l.Errorw("查询用户失败", logx.Field("user_id", req.Id), logx.Field("error", err))
		return &types.ApiResponse{
			Code:    404,
			Message: "用户不存在",
		}, nil
	}

	// 更新用户信息
	updateUser := &model.User{
		Id:       req.Id,
		Email:    req.Email,
	}

	if req.Nickname != "" {
		updateUser.Nickname.String = req.Nickname
		updateUser.Nickname.Valid = true
	} else {
		updateUser.Nickname.String = user.Username
		updateUser.Nickname.Valid = true
	}

	if err := l.svcCtx.UserModel.Update(updateUser); err != nil {
		l.Errorf("更新用户信息失败: %v", err)
		return &types.ApiResponse{
			Code:    500,
			Message: "更新失败",
		}, nil
	}

	// 获取更新后的用户信息
	updatedUser, err := l.svcCtx.UserModel.FindOne(req.Id)
	if err != nil {
		l.Errorf("获取更新后用户信息失败: %v", err)
		return &types.ApiResponse{
			Code:    500,
			Message: "更新成功，但获取信息失败",
		}, nil
	}

	// 处理昵称
	var nickname string
	if updatedUser.Nickname.Valid {
		nickname = updatedUser.Nickname.String
	} else {
		nickname = updatedUser.Username
	}

	userInfo := &types.UserInfoResponse{
		Id:       updatedUser.Id,
		Username: updatedUser.Username,
		Email:    updatedUser.Email,
		Nickname: nickname,
	}

	l.Infow("用户信息更新成功", logx.Field("user_id", req.Id))

	return &types.ApiResponse{
		Code:    200,
		Message: "更新成功",
		Data:    userInfo,
	}, nil
}
