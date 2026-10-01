// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	// MySQL 内容服务数据库。DSN 含 file: 或 .db 时走 SQLite（开发/测试），
	// 否则按 MySQL 连接（生产）；默认本地 SQLite 文件，零配置可跑。
	MySQL struct {
		DataSource string `json:",env=DATASOURCE,default=file:data/content.db"`
	}

	// JWT 配置
	Auth struct {
		JWTSecret string `json:",env=JWT_SECRET"`
	}

	// 外部服务配置
	Services struct {
		GameCatalog struct {
			BaseURL string `json:",default=http://localhost:8890,env=GAMECATALOG_BASE_URL"`
			Timeout int64  `json:",default=5000,env=GAMECATALOG_TIMEOUT"` // 毫秒
		}
	}
}
