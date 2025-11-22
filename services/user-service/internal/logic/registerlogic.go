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

type RegisterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RegisterLogic) Register(req *types.RegisterRequest) (resp *types.ApiResponse, err error) {
	// 验证输入数据
	if err := utils.ValidateUsername(req.Username); err != nil {
		return &types.ApiResponse{
			Code:    400,
			Message: err.Error(),
		}, nil
	}

	if err := utils.ValidateEmail(req.Email); err != nil {
		return &types.ApiResponse{
			Code:    400,
			Message: err.Error(),
		}, nil
	}

	if err := utils.ValidatePassword(req.Password); err != nil {
		return &types.ApiResponse{
			Code:    400,
			Message: err.Error(),
		}, nil
	}

	// 检查用户名是否已存在
	exists, err := l.svcCtx.UserModel.CheckUsernameExists(req.Username)
	if err != nil {
		l.Errorf("检查用户名失败: %v", err)
		return &types.ApiResponse{
			Code:    500,
			Message: "服务器内部错误",
		}, nil
	}
	if exists {
		return &types.ApiResponse{
			Code:    409,
			Message: "用户名已存在",
		}, nil
	}

	// 检查邮箱是否已存在
	exists, err = l.svcCtx.UserModel.CheckEmailExists(req.Email)
	if err != nil {
		l.Errorf("检查邮箱失败: %v", err)
		return &types.ApiResponse{
			Code:    500,
			Message: "服务器内部错误",
		}, nil
	}
	if exists {
		return &types.ApiResponse{
			Code:    409,
			Message: "邮箱已被注册",
		}, nil
	}

	// 加密密码
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		l.Errorf("密码加密失败: %v", err)
		return &types.ApiResponse{
			Code:    500,
			Message: "服务器内部错误",
		}, nil
	}

	// 创建用户
	user := &model.User{
		Username: req.Username,
		Email:    req.Email,
		Password: hashedPassword,
	}

	if req.Nickname != "" {
		user.Nickname.String = req.Nickname
		user.Nickname.Valid = true
	} else {
		user.Nickname.String = req.Username
		user.Nickname.Valid = true
	}

	if err := l.svcCtx.UserModel.Create(user); err != nil {
		l.Errorf("创建用户失败: %v", err)
		return &types.ApiResponse{
			Code:    500,
			Message: "注册失败",
		}, nil
	}

	l.Infow("用户注册成功", logx.Field("username", req.Username), logx.Field("user_id", user.Id))

	return &types.ApiResponse{
		Code:    200,
		Message: "注册成功",
		Data: map[string]interface{}{
			"user_id": user.Id,
		},
	}, nil
}
