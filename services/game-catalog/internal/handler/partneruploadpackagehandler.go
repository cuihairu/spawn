package handler

import (
	"io"
	"mime/multipart"
	"net/http"

	"github.com/tappi/tappi/services/game-catalog/internal/logic"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

// PartnerUploadPackageHandler GP-105 合作方上传平台托管包体（multipart，
// 文件字段 file）。不走 httpx.Parse：multipart 内容类型下 Parse 会先缓冲
// 表单，抢在流式读取前消费请求体。路径参数经 pathvar 直取，包体流直接
// 交给 logic 层落盘（上限 2 GiB 由 PackageStore 强制）。
func PartnerUploadPackageHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := pathvar.Vars(r)
		gameId, versionId := vars["id"], vars["versionId"]

		mr, err := r.MultipartReader()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w,
				logic.NewAPIError(http.StatusBadRequest, "需要 multipart/form-data 请求"))
			return
		}
		var fileName string
		var filePart *multipart.Part
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				httpx.ErrorCtx(r.Context(), w,
					logic.NewAPIError(http.StatusBadRequest, "解析 multipart 请求失败"))
				return
			}
			if part.FormName() == "file" {
				fileName = part.FileName()
				filePart = part
				break
			}
			// 非文件字段排空后继续，保证解析器状态一致
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
		}
		if filePart == nil {
			httpx.ErrorCtx(r.Context(), w,
				logic.NewAPIError(http.StatusBadRequest, "缺少文件字段 file"))
			return
		}
		defer filePart.Close()

		resp, err := logic.NewPartnerLogic(withClientIP(r), svcCtx).
			UploadPackage(gameId, versionId, fileName, filePart)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
