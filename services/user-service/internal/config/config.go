// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	// 数据库配置
	MySQL struct {
		DataSource string `json:",env=DATASOURCE"`
	}

	// JWT 配置
	Auth struct {
		JWTSecret   string `json:",env=JWT_SECRET"`
		TokenExpire int64  `json:",default=7"` // 令牌过期时间（天）
	}

	Services struct {
		GameCatalog struct {
			BaseURL string `json:",default=http://localhost:8890,env=GAMECATALOG_BASE_URL"`
			Timeout int64  `json:",default=5000,env=GAMECATALOG_TIMEOUT"` // 毫秒
		}
	}
}
