package logic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tappi/tappi/services/game-catalog/internal/analytics"
	"github.com/tappi/tappi/services/game-catalog/internal/store"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// DownloadResult 受控下载的已校验产物：版本记录 + 已打开的包体只读句柄。
// 文件句柄由 handler 层负责关闭（defer File.Close）。
type DownloadResult struct {
	Version *model.GameVersion
	File    *os.File
	Size    int64
	Ext     string
	ModTime time.Time
}

type DownloadVersionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDownloadVersionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DownloadVersionLogic {
	return &DownloadVersionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Download GP-105 受控下载读取侧：只放行「游戏匹配 + 已发布 + 平台托管」的
// 版本；不存在/未发布/跨游戏一律 404，不泄露版本存在性。包体未托管在平台
// （合作方只登记了外部 storage_url）返回 409，客户端改走商店链接（F08）。
func (l *DownloadVersionLogic) Download(gameId, versionId string) (*DownloadResult, error) {
	if l.svcCtx.PackageStore == nil {
		return nil, newAPIError(http.StatusInternalServerError, "包体存储未配置")
	}
	v, err := l.svcCtx.AdminRepository.GetVersion(versionId)
	if errors.Is(err, model.ErrVersionNotFound) {
		return nil, newAPIError(http.StatusNotFound, "版本不存在或未发布")
	}
	if err != nil {
		l.Errorf("get version failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询版本失败")
	}
	if v.GameId != gameId || v.Status != model.VersionStatusPublished {
		return nil, newAPIError(http.StatusNotFound, "版本不存在或未发布")
	}
	if !strings.HasPrefix(v.StorageUrl, store.PackageURLScheme) {
		return nil, newAPIError(http.StatusConflict, "包体未托管在平台，请使用存储地址或商店链接")
	}
	f, size, err := l.svcCtx.PackageStore.Open(v.Id)
	if errors.Is(err, store.ErrPackageNotFound) {
		l.Errorf("package file missing for %s (storage_url=%s)", v.Id, v.StorageUrl)
		return nil, newAPIError(http.StatusInternalServerError, "包体文件缺失")
	}
	if err != nil {
		l.Errorf("open package failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "读取包体失败")
	}
	// handler 关闭句柄前先解析扩展名/时间戳，避免用后读
	ext := strings.TrimPrefix(strings.TrimPrefix(v.StorageUrl, store.PackageURLScheme), v.Id)
	if !strings.HasPrefix(ext, ".") {
		ext = ".bin"
	}
	var modTime time.Time
	if t, err := time.Parse(time.RFC3339, v.CreatedAt); err == nil {
		modTime = t
	}
	l.emitDownload(gameId)
	return &DownloadResult{
		Version: v,
		File:    f,
		Size:    size,
		Ext:     ext,
		ModTime: modTime,
	}, nil
}

// emitDownload GP-402/F24：包体下载事实事件（漏斗阶段）。每次成功开启
// 下载记一条（下载可重复，故 event_id 用随机事实 ID 而非派生键）；
// fire-and-forget，不阻塞流式响应。
func (l *DownloadVersionLogic) emitDownload(gameId string) {
	if l.svcCtx.AnalyticsSvc == nil || !l.svcCtx.AnalyticsSvc.Enabled() {
		return
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return
	}
	l.svcCtx.AnalyticsSvc.Emit(analytics.Event{
		EventId:   "download:" + hex.EncodeToString(buf),
		EventType: analytics.EventDownload,
		UserId:    l.userID(),
		GameId:    gameId,
	})
}

func (l *DownloadVersionLogic) userID() int64 {
	if id, ok := l.ctx.Value("user_id").(int64); ok {
		return id
	}
	return 0
}
