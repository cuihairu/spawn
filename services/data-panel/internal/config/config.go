// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	// MySQL data-panel 服务数据库。DSN 含 file: 或 .db 时走 SQLite（开发/测试），
	// 否则按 MySQL 连接（生产）；默认本地 SQLite 文件，零配置可跑。
	MySQL struct {
		DataSource string `json:",env=DATASOURCE,default=file:data/stats.db"`
	}

	// Auth JWT 校验配置（与 user-service 共享密钥，令牌由 user-service 签发）
	Auth struct {
		JWTSecret string `json:",env=JWT_SECRET"`
	}
}
