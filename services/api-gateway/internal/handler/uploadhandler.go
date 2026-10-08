package handler

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/tappi/tappi/services/api-gateway/internal/upload"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

// UploadHandler 统一上传入口（POST /upload，JWT 之后）：图片经魔数校验后
// 落本地盘（选型方案 A），返回 "/uploads/..." URL，业务服务只存 URL。
type UploadResponse struct {
	Url string `json:"url"`
}

func UploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r.Header.Get("Authorization"))
		if token == "" {
			httpx.WriteJsonCtx(r.Context(), w, http.StatusUnauthorized,
				map[string]string{"error": "缺少访问令牌"})
			return
		}
		if _, err := svcCtx.UploadAuth.ParseToken(token); err != nil {
			httpx.WriteJsonCtx(r.Context(), w, http.StatusUnauthorized,
				map[string]string{"error": "无效的访问令牌"})
			return
		}

		url, err := svcCtx.UploadStore.SaveImage(r, "file")
		if err != nil {
			var tooLarge *upload.MaxUploadError
			if errors.As(err, &tooLarge) {
				httpx.WriteJsonCtx(r.Context(), w, http.StatusRequestEntityTooLarge,
					map[string]string{"error": err.Error()})
				return
			}
			httpx.WriteJsonCtx(r.Context(), w, http.StatusBadRequest,
				map[string]string{"error": err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, UploadResponse{Url: url})
	}
}

// UploadsFileHandler 静态托管已上传图片（GET /uploads/:file）：文件名过
// 白名单后再拼接路径，杜绝路径遍历与任意文件读取。
func UploadsFileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := pathvar.Vars(r)
		target := upload.SafeResolve(svcCtx.UploadStore.Dir, vars["file"])
		if target == "" {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(target)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		http.ServeContent(w, r, filepath.Base(target), time.Time{}, f)
	}
}
