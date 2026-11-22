package handler

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/tappi/tappi/services/game-catalog/internal/logic"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// DownloadVersionHandler GP-105 受控下载：流式返回平台托管的已发布包体。
// http.ServeContent 处理 Range（断点续传/If-Range）；X-Package-Sha256 供
// 客户端校验完整性（F08）。仅登录用户可下载，鉴权在路由中间件。
func DownloadVersionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.VersionPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		res, err := logic.NewDownloadVersionLogic(withClientIP(r), svcCtx).Download(req.Id, req.VersionId, req.Channel)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		defer res.File.Close()

		w.Header().Set("X-Package-Sha256", strings.TrimPrefix(res.Version.Hash, "sha256:"))
		w.Header().Set("Content-Disposition",
			"attachment; filename=\""+attachmentName(res.Version.PackageName, res.Version.Version, res.Version.Id, res.Ext)+"\"")
		switch res.Ext {
		case ".apk":
			w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		default:
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		// ETag 用内容哈希：换包后 If-Range 自动失效，续传不会拼到旧包片段
		w.Header().Set("ETag", "\""+strings.TrimPrefix(res.Version.Hash, "sha256:")+"\"")
		http.ServeContent(w, r, filepath.Base(res.Version.Id+res.Ext), res.ModTime, res.File)
	}
}

// attachmentName 下载文件名：包名-版本-版本id<ext>；包名缺省回退游戏 id，
// 顺手替换引号/斜杠等字符，避免 Content-Disposition 注入。
func attachmentName(packageName, version, versionId, ext string) string {
	base := strings.TrimSpace(packageName)
	if base == "" {
		base = "package"
	}
	name := base + "-" + version + "-" + versionId + ext
	return strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r == '/' || r < 0x20 {
			return '-'
		}
		return r
	}, name)
}
