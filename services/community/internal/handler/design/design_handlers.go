// Code scaffolded by goctl. Safe to edit.
// DESIGN-001 设计拆解域 handlers：公开读走 httpx.Parse；写端点鉴权在
// Auth 中间件 + logic 层（作者/管理员判定）。

package design

import (
	"net/http"

	"github.com/tappi/tappi/services/community/internal/logic/design"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ListCategoriesHandler 品类入口（公开读）。
func ListCategoriesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := design.NewDesignLogic(r.Context(), svcCtx)
		if resp, err := l.Categories(); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// ListBreakdownsHandler 拆解列表（公开读，品类/精选筛选 + 分页）。
func ListBreakdownsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DesignListReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := design.NewDesignLogic(r.Context(), svcCtx)
		if resp, err := l.List(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// GetBreakdownHandler 拆解详情（公开读，全文）。
func GetBreakdownHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DesignPathReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := design.NewDesignLogic(r.Context(), svcCtx)
		if resp, err := l.Get(req.Id); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// CreateBreakdownHandler 发布拆解（登录即可）。
func CreateBreakdownHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DesignCreateReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := design.NewDesignLogic(r.Context(), svcCtx)
		if resp, err := l.Create(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// UpdateBreakdownHandler 编辑拆解（仅作者）。
func UpdateBreakdownHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DesignUpdateReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := design.NewDesignLogic(r.Context(), svcCtx)
		if resp, err := l.Update(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// FeatureBreakdownHandler 精选运营位（仅审核管理员）。
func FeatureBreakdownHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DesignFeatureReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := design.NewDesignLogic(r.Context(), svcCtx)
		if err := l.SetFeatured(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "ok"})
		}
	}
}

// ListDesignCommentsHandler 讨论区评论列表（公开读）。
func ListDesignCommentsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DesignCommentsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := design.NewDesignLogic(r.Context(), svcCtx)
		if resp, err := l.ListComments(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// CreateDesignCommentHandler 讨论区发言（登录）。
func CreateDesignCommentHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DesignCommentCreateReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := design.NewDesignLogic(r.Context(), svcCtx)
		if resp, err := l.AddComment(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, map[string]any{"comment": resp})
		}
	}
}

// DeleteDesignCommentHandler 删除自己的评论。
func DeleteDesignCommentHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DesignCommentDeleteReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := design.NewDesignLogic(r.Context(), svcCtx)
		if err := l.DeleteComment(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "ok"})
		}
	}
}
