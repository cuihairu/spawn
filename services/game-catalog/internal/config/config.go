// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	// DataSource 配置了游戏数据文件或外部数据源
	DataSource struct {
		File string `json:",default=data/games.json"`
	}

	// Auth JWT 校验配置（与 user-service 共享密钥，令牌由 user-service 签发）
	Auth struct {
		JWTSecret string `json:",env=JWT_SECRET"`
	}
}
