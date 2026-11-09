// Package store 包体存储（GP-105）：本地磁盘托管合作方上传的渠道包，
// 按版本 id 组织，供受控下载端点只读分发。生产部署须挂持久卷并纳入备份
// （接入设计 §4 持久化方案）；对象存储（OSS/S3）实现留作后续替换，
// 调用方只依赖本文件暴露的窄接口。
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MaxPackageBytes 单包上限：渠道包通常数百 MB，2 GiB 覆盖合理包体，
	// 防止异常上传塞满磁盘（超限在流式落盘中途截断，不整体驻留内存）。
	MaxPackageBytes = int64(2) << 30

	// PackageURLScheme 平台托管包体的 storage_url 协议头：
	// package://<versionId><ext>，受控下载仅放行该协议的版本。
	PackageURLScheme = "package://"
)

var (
	ErrPackageTooLarge = errors.New("package exceeds size limit")
	ErrPackageNotFound = errors.New("package not found")
)

// LocalPackageStore 本地磁盘包体存储；并发安全（写走临时文件+原子重命名）。
type LocalPackageStore struct {
	dir      string
	maxBytes int64
}

// NewLocalPackageStore 创建存储根目录（幂等）；目录不可写时快速失败。
func NewLocalPackageStore(dir string) (*LocalPackageStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("包体存储目录未配置")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建包体存储目录失败: %w", err)
	}
	return &LocalPackageStore{dir: dir, maxBytes: MaxPackageBytes}, nil
}

// Save 流式落盘包体并返回（大小, sha256hex, 净化后的扩展名）。
// 文件名只取扩展名（字母数字、≤10 字符，缺省 .bin），落盘名为 <versionId><ext>；
// 先写临时文件再原子重命名，半截包不会被下载端点读到；同版本旧扩展名的
// 遗留文件顺手清理（换包后只保留最新一份）。
func (s *LocalPackageStore) Save(versionID, fileName string, r io.Reader) (int64, string, string, error) {
	ext := sanitizeExt(fileName)
	tmp, err := os.CreateTemp(s.dir, ".upload-*")
	if err != nil {
		return 0, "", "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpName) }

	hasher := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(r, s.maxBytes+1))
	if copyErr == nil && n > s.maxBytes {
		copyErr = ErrPackageTooLarge
	}
	if copyErr != nil {
		cleanup()
		return 0, "", "", copyErr
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return 0, "", "", fmt.Errorf("写入包体失败: %w", err)
	}
	final := filepath.Join(s.dir, versionID+ext)
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return 0, "", "", fmt.Errorf("落盘包体失败: %w", err)
	}
	// 清理同版本其他扩展名的旧包（best effort，失败不影响本次结果）
	if stale, _ := filepath.Glob(filepath.Join(s.dir, versionID+".*")); len(stale) > 0 {
		for _, p := range stale {
			if p != final {
				_ = os.Remove(p)
			}
		}
	}
	return n, hex.EncodeToString(hasher.Sum(nil)), ext, nil
}

// Open 打开版本对应的托管包体（只读）并返回大小。
// versionId 来自库内记录（ver-uuid 格式），仍防御性拒绝路径分隔符。
func (s *LocalPackageStore) Open(versionID string) (*os.File, int64, error) {
	if versionID == "" || strings.ContainsAny(versionID, "/\\") ||
		strings.ContainsAny(versionID, "*?[]") {
		return nil, 0, ErrPackageNotFound
	}
	matches, err := filepath.Glob(filepath.Join(s.dir, versionID+".*"))
	if err != nil || len(matches) == 0 {
		return nil, 0, ErrPackageNotFound
	}
	f, err := os.Open(matches[0])
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, ErrPackageNotFound
		}
		return nil, 0, fmt.Errorf("打开包体失败: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, fmt.Errorf("读取包体大小失败: %w", err)
	}
	return f, st.Size(), nil
}

// sanitizeExt 取上传文件名的扩展名：小写、字母数字、1–10 字符（不含点），
// 其余一律回退 .bin，避免奇怪后缀进 Content-Disposition/Content-Type。
func sanitizeExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	body := strings.TrimPrefix(ext, ".")
	if body == "" || len(body) > 10 {
		return ".bin"
	}
	for _, ch := range body {
		if !(ch >= 'a' && ch <= 'z') && !(ch >= '0' && ch <= '9') {
			return ".bin"
		}
	}
	return ext
}
