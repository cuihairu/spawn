package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	// 数据库配置（DSN 含 file:/.db 走 SQLite，否则 MySQL，与 payments 同款探测）
	MySQL struct {
		DataSource string `json:",env=DATASOURCE"`
	}

	// 平台 JWT 校验密钥（与 user-service 共享）
	Auth struct {
		JWTSecret string `json:",env=JWT_SECRET"`
	}

	// GameCatalog 角色权威（/me）：活动配置管理面按平台角色放行
	GameCatalog struct {
		BaseURL string `json:",default=http://localhost:8890,env=GAMECATALOG_BASE_URL"`
		Timeout int64  `json:",default=5000,env=GAMECATALOG_TIMEOUT"`
	}
}
