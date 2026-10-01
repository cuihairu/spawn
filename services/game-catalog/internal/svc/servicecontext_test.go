package svc

import (
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
)

// TestNewServiceContext_RelativeDataSource 相对路径数据源走 filepath.Clean
// 归一化分支；文件不存在时仓储回落内置种子数据。
func TestNewServiceContext_RelativeDataSource(t *testing.T) {
	var c config.Config
	c.DataSource.File = "nonexistent-relative-games.json"
	c.Auth.JWTSecret = "svc-test-secret"

	ctx := NewServiceContext(c)

	if ctx.GameRepository == nil || ctx.Auth == nil {
		t.Fatalf("incomplete context: %+v", ctx)
	}
	game, err := ctx.GameRepository.Get("game-elden-ring")
	if err != nil || game == nil || game.Title != "Elden Ring" {
		t.Fatalf("expected embedded seed fallback, got game=%+v err=%v", game, err)
	}
}
