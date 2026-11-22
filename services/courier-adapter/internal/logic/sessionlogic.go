package logic

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/tappi/tappi/services/courier-adapter/courier"
	"github.com/tappi/tappi/services/courier-adapter/internal/svc"
	"github.com/tappi/tappi/services/courier-adapter/internal/ticket"
	"github.com/tappi/tappi/services/courier-adapter/internal/types"
	"github.com/tappi/tappi/services/courier-adapter/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type SessionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSessionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SessionLogic {
	return &SessionLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// GameSession 进入游戏（接入设计 §1.3）：校验映射与能力 → 查/建绑定
// （无绑定静默建 Courier 账号）→ 代登录该 scope → 签发一次性启动票据。
func (l *SessionLogic) GameSession(spawnGameID, authHeader string, userID int64) (*types.GameSessionResponse, error) {
	mapping, err := l.svcCtx.Mappings.Get(spawnGameID)
	if err != nil {
		if errors.Is(err, model.ErrMappingNotFound) {
			return nil, newAPIError(http.StatusNotFound, "游戏未接入 Courier")
		}
		return nil, newAPIError(http.StatusInternalServerError, "查询映射失败")
	}
	if !mapping.Capabilities.Login {
		return nil, newAPIError(http.StatusForbidden, "该游戏未开通游戏内登录")
	}

	sess, err := l.EnsureSession(mapping, authHeader, userID)
	if err != nil {
		return nil, err
	}

	t, err := l.svcCtx.Tickets.Issue(userID, mapping.CourierGameId, mapping.Env, sess)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "签发票据失败")
	}
	return &types.GameSessionResponse{
		Ticket:    t.Token,
		ExpiresAt: t.ExpiresAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		GameId:    mapping.CourierGameId,
		Env:       mapping.Env,
	}, nil
}

// EnsureSession 查/建绑定（无绑定静默建 Courier 账号）→ 代登录该 scope
// 返回会话。GP-201 游戏会话与 GP-307 售后工单共用同一绑定/代登录路径。
func (l *SessionLogic) EnsureSession(mapping *model.GameMapping, authHeader string, userID int64) (*courier.Session, error) {
	binding, err := l.svcCtx.Bindings.GetByUser(userID)
	switch {
	case errors.Is(err, model.ErrBindingNotFound):
		// 首次进入：静默建号（register 即登录该 scope），直接复用注册会话
		_, sess, err := l.autoBind(mapping, authHeader, userID)
		if err != nil {
			return nil, err
		}
		return sess, nil
	case err != nil:
		return nil, newAPIError(http.StatusInternalServerError, "查询绑定失败")
	default:
		return l.proxyLogin(mapping, binding, userID)
	}
}

// autoBind 无绑定时的静默建号：随机密码（AES-GCM 加密落库，不存明文）、
// 平台邮箱或占位邮箱，注册即登录该 scope，写绑定 + 审计；返回注册会话
// 供首进游戏直接签发票据（省一次代登录）。
func (l *SessionLogic) autoBind(mapping *model.GameMapping, authHeader string, userID int64) (*model.Binding, *courier.Session, error) {
	password, err := model.GenerateAppPassword()
	if err != nil {
		return nil, nil, newAPIError(http.StatusInternalServerError, "生成凭据失败")
	}
	email := fetchUserEmail(l.ctx, l.svcCtx, authHeader, userID)
	if email == "" {
		email = placeholderEmail(userID)
	}
	sess, err := l.svcCtx.Courier.Register(l.ctx, mapping.CourierGameId, mapping.Env, email, password)
	if err != nil {
		l.Errorf("courier register failed (user %d game %s): %v", userID, mapping.SpawnGameId, err)
		return nil, nil, courierUpstreamError("courier 建号失败", err)
	}
	enc, err := model.EncryptSecret(l.svcCtx.Config.Credential.Key, password)
	if err != nil {
		return nil, nil, newAPIError(http.StatusInternalServerError, "凭据加密失败")
	}
	binding := &model.Binding{
		SpawnUserID:      userID,
		CourierAccountID: sess.AccountID,
		CourierEmail:     email,
		SecretEnc:        enc,
		Status:           model.BindingActive,
		BindMethod:       "auto",
	}
	if err := l.svcCtx.Bindings.Create(binding); err != nil {
		l.Errorf("save binding failed (user %d): %v", userID, err)
		return nil, nil, newAPIError(http.StatusInternalServerError, "保存绑定失败")
	}
	if err := l.svcCtx.Audits.Insert(userID, "courier_bind", "binding", fmt.Sprintf("user:%d", userID),
		fmt.Sprintf(`{"courierAccountId":%q,"spawnGameId":%q,"method":"auto"}`, sess.AccountID, mapping.SpawnGameId), clientIP(l.ctx)); err != nil {
		l.Errorf("audit courier_bind failed: %v", err)
	}
	return binding, sess, nil
}

// proxyLogin 代登录（已绑定路径；账号为公司级，密码密文解出后用完即弃）。
func (l *SessionLogic) proxyLogin(mapping *model.GameMapping, binding *model.Binding, userID int64) (*courier.Session, error) {
	password, err := model.DecryptSecret(l.svcCtx.Config.Credential.Key, binding.SecretEnc)
	if err != nil {
		l.Errorf("decrypt courier secret failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "绑定凭据解密失败")
	}
	sess, err := l.svcCtx.Courier.Login(l.ctx, mapping.CourierGameId, mapping.Env, binding.CourierEmail, password)
	if err != nil {
		l.Errorf("courier login failed (user %d game %s): %v", userID, mapping.SpawnGameId, err)
		return nil, courierUpstreamError("courier 代登录失败", err)
	}
	return sess, nil
}

// Redeem 票据兑换 scope-bound token（一次性；票据即凭证，无需平台 JWT）。
func (l *SessionLogic) Redeem(token string) (*types.RedeemResponse, error) {
	t, err := l.svcCtx.Tickets.Redeem(token)
	if err != nil {
		if errors.Is(err, ticket.ErrTicketNotFound) {
			return nil, newAPIError(http.StatusNotFound, "票据不存在、已兑换或已过期")
		}
		return nil, newAPIError(http.StatusInternalServerError, "兑换失败")
	}
	sess := t.Session
	if sess == nil {
		return nil, newAPIError(http.StatusInternalServerError, "票据会话缺失")
	}
	return &types.RedeemResponse{
		Account: types.RedeemAccount{
			Id:     sess.AccountID,
			Type:   sess.AccountType,
			Status: sess.AccountStatus,
		},
		AccessToken:      sess.AccessToken,
		AccessExpiresAt:  sess.AccessExpiresAt,
		RefreshToken:     sess.RefreshToken,
		RefreshExpiresAt: sess.RefreshExpiresAt,
	}, nil
}

// courierUpstreamError Courier 上游错误的统一呈现：域错误码透传语义，
// 其余归 502（网关上游故障），不吞细节进响应。
func courierUpstreamError(prefix string, err error) error {
	var apiErr *courier.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "AUTH_ACCOUNT_PENDING_DELETE":
			return newAPIError(http.StatusConflict, "Courier 账号注销宽限期内，请先撤销注销")
		case "AUTH_ACCOUNT_DISABLED":
			return newAPIError(http.StatusForbidden, "Courier 账号已禁用")
		}
	}
	return newAPIError(http.StatusBadGateway, prefix)
}

func clientIP(ctx context.Context) string {
	// handler 侧以 client_ip 注入；logic 侧兜底空值。
	if v := ctx.Value("client_ip"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
