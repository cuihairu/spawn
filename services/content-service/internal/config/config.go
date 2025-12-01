// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	// DataSource 配置攻略和评论数据文件
	DataSource struct {
		GuidesFile   string `json:",default=data/guides.json"`
		CommentsFile string `json:",default=data/comments.json"`
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
