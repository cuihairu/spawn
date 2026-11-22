package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalPackageStore_SaveOpen(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalPackageStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	payload := []byte("apk-bytes-0123456789")
	size, hash, ext, err := s.Save("ver-1", "demo.APK", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if size != int64(len(payload)) || ext != ".apk" {
		t.Fatalf("save = size %d ext %s, want %d/.apk", size, ext, len(payload))
	}
	sum := sha256.Sum256(payload)
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash = %s", hash)
	}

	f, openSize, err := s.Open("ver-1")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	if openSize != int64(len(payload)) {
		t.Fatalf("open size = %d, want %d", openSize, len(payload))
	}
	got, err := io.ReadAll(f)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("roundtrip mismatch (err=%v)", err)
	}

	// 换包（不同扩展名）：旧包清理，只留最新
	if _, _, ext2, err := s.Save("ver-1", "demo.bin", bytes.NewReader(payload)); err != nil || ext2 != ".bin" {
		t.Fatalf("resave: %v ext=%s", err, ext2)
	}
	f2, _, err := s.Open("ver-1")
	if err != nil {
		t.Fatalf("reopen after resave: %v", err)
	}
	_ = f2.Close()

	if _, _, err := s.Open("ver-missing"); !errors.Is(err, ErrPackageNotFound) {
		t.Fatalf("open missing = %v, want ErrPackageNotFound", err)
	}
}

func TestLocalPackageStore_TooLarge(t *testing.T) {
	// 直接构造小上限实例（同包测试钩子），验证流式截断而不用真写 2 GiB。
	s := &LocalPackageStore{dir: t.TempDir(), maxBytes: 8}
	_, _, _, err := s.Save("ver-2", "big.apk", bytes.NewReader(make([]byte, 9)))
	if !errors.Is(err, ErrPackageTooLarge) {
		t.Fatalf("save oversize = %v, want ErrPackageTooLarge", err)
	}
	if _, _, err := s.Open("ver-2"); !errors.Is(err, ErrPackageNotFound) {
		t.Fatalf("oversize must not leave a file, got %v", err)
	}
}

func TestLocalPackageStore_SanitizeExt(t *testing.T) {
	cases := map[string]string{
		"demo.apk":          ".apk",
		"DEMO.Apk":          ".apk",
		"a.b.c.zip":         ".zip",
		"noext":             ".bin",
		"evil.sh#":          ".bin",
		"../../x.apk":       ".apk",
		"long.a23456789012": ".bin", // 扩展名主体 >10 字符
		"x":                 ".bin",
	}
	for in, want := range cases {
		if got := sanitizeExt(in); got != want {
			t.Fatalf("sanitizeExt(%q) = %q, want %q", in, got, want)
		}
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// NewLocalPackageStore 失败臂：空目录、父路径不可用（MkdirAll 失败）。
func TestLocalPackageStore_NewErrors(t *testing.T) {
	if _, err := NewLocalPackageStore(""); err == nil {
		t.Fatal("NewLocalPackageStore('') = nil error, want error")
	}
	if _, err := NewLocalPackageStore("/dev/null/x"); err == nil {
		t.Fatal("NewLocalPackageStore under file path = nil error, want error")
	}
}

// Save 失败臂：CreateTemp 目录不存在、Rename 目标目录不存在（versionID 含分隔符）。
func TestLocalPackageStore_SaveErrors(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalPackageStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	missing := &LocalPackageStore{dir: filepath.Join(dir, "nope"), maxBytes: 1024}
	if _, _, _, err := missing.Save("v", "f.apk", bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("Save on missing dir = nil error, want CreateTemp error")
	}
	if _, _, _, err := s.Save("sub/v", "f.apk", bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("Save with slash versionID = nil error, want rename error")
	}
}

// Open 失败臂：非法 versionID 拒绝；悬空链接 → ENOENT → ErrPackageNotFound；
// 符号链接环 → ELOOP → 通用错误（非 ErrPackageNotFound）。
func TestLocalPackageStore_OpenErrors(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalPackageStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, _, err := s.Open("bad/id"); !errors.Is(err, ErrPackageNotFound) {
		t.Fatalf("Open(bad/id) = %v, want ErrPackageNotFound", err)
	}
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "v1.bin")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, _, err := s.Open("v1"); !errors.Is(err, ErrPackageNotFound) {
		t.Fatalf("Open(dangling symlink) = %v, want ErrPackageNotFound", err)
	}
	loop := filepath.Join(dir, "v2.bin")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatalf("symlink loop: %v", err)
	}
	if _, _, err := s.Open("v2"); err == nil || errors.Is(err, ErrPackageNotFound) {
		t.Fatalf("Open(symlink loop) = %v, want generic error", err)
	}
}
