// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package logic

import (
	"context"
	"errors"

	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/internal/types"
	"github.com/tappi/tappi/services/user-service/utils"

	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LoginLogic) Login(req *types.LoginRequest) (resp *types.LoginResponse, err error) {
	// 验证输入数据
	if req.Username == "" {
		return nil, errors.New("用户名不能为空")
	}

	if req.Password == "" {
		return nil, errors.New("密码不能为空")
	}

	// 查找用户
	user, err := l.svcCtx.UserModel.FindByUsername(req.Username)
	if err != nil {
		l.Errorw("用户不存在", logx.Field("username", req.Username))
		return nil, errors.New("用户名或密码错误")
	}

	// 验证密码
	if !utils.CheckPassword(req.Password, user.Password) {
		l.Errorw("密码验证失败", logx.Field("username", req.Username))
		return nil, errors.New("用户名或密码错误")
	}

	// 生成JWT令牌
	token, err := l.svcCtx.Auth.GenerateToken(user.Id, user.Username)
	if err != nil {
		l.Errorf("生成令牌失败: %v", err)
		return nil, errors.New("服务器内部错误")
	}

	// 处理昵称
	var nickname string
	if user.Nickname.Valid {
		nickname = user.Nickname.String
	} else {
		nickname = user.Username
	}

	l.Infow("用户登录成功", logx.Field("username", req.Username), logx.Field("user_id", user.Id))

	return &types.LoginResponse{
		Token: token,
		UserInfo: &types.UserInfoResponse{
			Id:       user.Id,
			Username: user.Username,
			Email:    user.Email,
			Nickname: nickname,
		},
	}, nil
}
