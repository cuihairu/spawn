package logic

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/tappi/tappi/services/courier-adapter/internal/svc"
	"github.com/tappi/tappi/services/courier-adapter/internal/types"
	"github.com/tappi/tappi/services/courier-adapter/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type AccountLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAccountLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AccountLogic {
	return &AccountLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// Delete 注销同步（接入设计 §1.3 步骤 9）：代登录 → Courier
// account/delete → 绑定 PENDING_DELETE（宽限期）或 UNBOUND（立即删除）。
func (l *AccountLogic) Delete(spawnGameID, authHeader string, userID int64) (*types.DeleteResultResponse, error) {
	mapping, binding, err := l.mappingAndBinding(spawnGameID, userID)
	if err != nil {
		return nil, err
	}
	sess, err := l.proxyLogin(mapping, binding, userID)
	if err != nil {
		return nil, err
	}
	res, err := l.svcCtx.Courier.DeleteAccount(l.ctx, mapping.CourierGameId, mapping.Env, sess)
	if err != nil {
		l.Errorf("courier account/delete failed (user %d): %v", userID, err)
		return nil, courierUpstreamError("courier 注销受理失败", err)
	}
	out := &types.DeleteResultResponse{Status: res.Status, CancelToken: res.CancelToken,
		PendingDelete: res.PendingDelete, DeletedAt: res.DeletedAt}
	switch res.Status {
	case "DELETED":
		// 立即删除（GUEST/宽限期=0）：绑定终态 UNBOUND，保留审计可追踪
		if err := l.svcCtx.Bindings.UpdateStatus(userID, model.BindingUnbound); err != nil {
			l.Errorf("mark binding unbound failed (user %d): %v", userID, err)
		}
	default:
		if err := l.svcCtx.Bindings.UpdateStatus(userID, model.BindingPendingDelete); err != nil {
			l.Errorf("mark binding pending delete failed (user %d): %v", userID, err)
		}
	}
	if err := l.svcCtx.Audits.Insert(userID, "courier_account_delete", "binding",
		fmt.Sprintf("user:%d", userID), fmt.Sprintf(`{"status":%q,"courierAccountId":%q}`, res.Status, binding.CourierAccountID), clientIP(l.ctx)); err != nil {
		l.Errorf("audit courier_account_delete failed: %v", err)
	}
	return out, nil
}

// CancelDelete 宽限期撤销：Courier cancel → 绑定回 ACTIVE。
func (l *AccountLogic) CancelDelete(spawnGameID, cancelToken string, userID int64) error {
	if cancelToken == "" {
		return newAPIError(http.StatusBadRequest, "cancelToken 不能为空")
	}
	mapping, binding, err := l.mappingAndBinding(spawnGameID, userID)
	if err != nil {
		return err
	}
	if err := l.svcCtx.Courier.CancelDelete(l.ctx, mapping.CourierGameId, mapping.Env, cancelToken); err != nil {
		l.Errorf("courier delete cancel failed (user %d): %v", userID, err)
		return courierUpstreamError("courier 撤销注销失败", err)
	}
	if err := l.svcCtx.Bindings.UpdateStatus(userID, model.BindingActive); err != nil {
		l.Errorf("reactivate binding failed (user %d): %v", userID, err)
	}
	if err := l.svcCtx.Audits.Insert(userID, "courier_account_delete_cancel", "binding",
		fmt.Sprintf("user:%d", userID), fmt.Sprintf(`{"courierAccountId":%q}`, binding.CourierAccountID), clientIP(l.ctx)); err != nil {
		l.Errorf("audit cancel failed: %v", err)
	}
	return nil
}

func (l *AccountLogic) mappingAndBinding(spawnGameID string, userID int64) (*model.GameMapping, *model.Binding, error) {
	mapping, err := l.svcCtx.Mappings.Get(spawnGameID)
	if err != nil {
		if errors.Is(err, model.ErrMappingNotFound) {
			return nil, nil, newAPIError(http.StatusNotFound, "游戏未接入 Courier")
		}
		return nil, nil, newAPIError(http.StatusInternalServerError, "查询映射失败")
	}
	binding, err := l.svcCtx.Bindings.GetByUser(userID)
	if err != nil {
		if errors.Is(err, model.ErrBindingNotFound) {
			return nil, nil, newAPIError(http.StatusNotFound, "尚未绑定 Courier 账号")
		}
		return nil, nil, newAPIError(http.StatusInternalServerError, "查询绑定失败")
	}
	return mapping, binding, nil
}

// proxyLogin 代登录（注销/撤销前取 Bearer 会话；账号为公司级，
// 任意已接入游戏 scope 的会话即可操作注销）。
func (l *AccountLogic) proxyLogin(mapping *model.GameMapping, binding *model.Binding, userID int64) (string, error) {
	password, err := model.DecryptSecret(l.svcCtx.Config.Credential.Key, binding.SecretEnc)
	if err != nil {
		l.Errorf("decrypt courier secret failed: %v", err)
		return "", newAPIError(http.StatusInternalServerError, "绑定凭据解密失败")
	}
	sess, err := l.svcCtx.Courier.Login(l.ctx, mapping.CourierGameId, mapping.Env, binding.CourierEmail, password)
	if err != nil {
		l.Errorf("courier login failed (user %d game %s): %v", userID, mapping.SpawnGameId, err)
		return "", courierUpstreamError("courier 代登录失败", err)
	}
	return sess.AccessToken, nil
}
