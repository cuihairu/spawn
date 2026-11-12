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

	// GP-402 经营分析事实投递（F24）：register/login 事实事件 → data-panel
	// /internal/events。BaseURL 未配置 = no-op（数据面板未部署时行为不变）。
	Analytics struct {
		BaseURL string `json:",env=DATA_PANEL_URL,optional"`
		Token   string `json:",env=DATA_PANEL_INTERNAL_TOKEN,optional"`
		Timeout int64  `json:",default=3000,env=DATA_PANEL_TIMEOUT"`
	}
}
