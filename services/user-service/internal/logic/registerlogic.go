// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package logic

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/analytics"
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

	// GP-403/F22：渠道归因。register 请求携带 click_id 时向 game-catalog
	// 解析点击所属推广渠道，只有落在归因窗口内才认领（一个点击只归因一次，
	// 不改写已归属的点击）。任何失败都只记日志，注册结果不受影响。
	occurredAt := time.Now().UTC().Format(time.RFC3339)
	channel := l.resolveChannel(req.ClickId, occurredAt, user.Id)

	// GP-403 首次活跃渠道归因：注册归因渠道持久化到用户行（登录事实事件
	// 读取后携带，支持渠道漏斗的首次活跃段）。空串（自然流量）不写；
	// 写失败仅记日志，不影响注册结果。
	if channel != "" {
		if err := l.svcCtx.UserModel.UpdatePromoChannel(user.Id, channel); err != nil {
			l.Errorf("持久化归因渠道失败 user_id=%d: %v", user.Id, err)
		}
	}

	// GP-402/F24：注册事实事件（漏斗首阶段）。event_id 由用户 ID 派生，
	// 同一用户重放只算一次注册；fire-and-forget 不影响注册结果。
	if l.svcCtx.AnalyticsSvc != nil && l.svcCtx.AnalyticsSvc.Enabled() {
		l.svcCtx.AnalyticsSvc.Emit(analytics.Event{
			EventId:    "register:" + strconv.FormatInt(user.Id, 10),
			EventType:  analytics.EventRegister,
			UserId:     user.Id,
			Channel:    channel,
			OccurredAt: occurredAt,
		})
	}

	return &types.ApiResponse{
		Code:    200,
		Message: "注册成功",
		Data: map[string]interface{}{
			"user_id": user.Id,
		},
	}, nil
}

// resolveChannel GP-403/F22：按 click_id 解析注册来源渠道。返回渠道标识；
// click_id 为空、解析失败或注册时刻已超出该渠道归因窗口时返回空串（自然流量）。
// 归因是尽力而为的旁路，绝不因解析失败影响注册成功与否。
//
// 补偿（本地 outbox）：归因意图先落 promo_attribution_outbox 再同步首投，
// game-catalog 不可达时失败留 pending 由 Relay 重投（此前同步失败即永久丢归因）。
// 重投安全性：game-catalog 侧窗口判定锚定原始 registered_at、回写 user_id 幂等
// （一点击一归因），重复解析不重复计、不抢注他人归因。
func (l *RegisterLogic) resolveChannel(clickID, registeredAt string, userID int64) string {
	clickID = strings.TrimSpace(clickID)
	if clickID == "" || l.svcCtx.GameCatalogClient == nil {
		return ""
	}
	outbox := l.svcCtx.PromoAttrOutbox
	attrKey := "attr:" + clickID + ":" + strconv.FormatInt(userID, 10)
	if outbox != nil {
		payload, _ := json.Marshal(map[string]any{
			"click_id":      clickID,
			"registered_at": registeredAt,
			"user_id":       userID,
		})
		now := time.Now().UTC().Format(time.RFC3339)
		if err := outbox.Enqueue(attrKey, clickID, userID, registeredAt, string(payload), now); err != nil {
			l.Errorf("归因意图入队失败 click_id=%s: %v", clickID, err)
		}
	}
	res, err := l.svcCtx.GameCatalogClient.ResolvePromoClick(l.ctx, clickID, registeredAt, userID)
	if err != nil {
		l.Errorf("解析推广点击失败 click_id=%s: %v", clickID, err)
		return ""
	}
	if outbox != nil {
		// 2xx 即下游受理（无论是否命中窗口，都是确定结果，无需重投）
		if err := outbox.MarkDelivered(attrKey, time.Now().UTC().Format(time.RFC3339)); err != nil {
			l.Errorf("归因意图标记已投失败 click_id=%s: %v", clickID, err)
		}
	}
	if res == nil || res.Channel == "" {
		return ""
	}
	if !res.InWindow {
		l.Infow("推广点击超出归因窗口，按自然流量处理",
			logx.Field("click_id", clickID), logx.Field("channel", res.Channel))
		return ""
	}
	return res.Channel
}
