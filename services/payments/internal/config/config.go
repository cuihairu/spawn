package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	// 数据库配置（DSN 含 file:/.db 走 SQLite，否则 MySQL，与 game-catalog 同款探测）
	MySQL struct {
		DataSource string `json:",env=DATASOURCE"`
	}

	// 平台 JWT 校验密钥（与 user-service 共享）
	Auth struct {
		JWTSecret string `json:",env=JWT_SECRET"`
	}

	// Courier 支付网关（契约 Frozen payment v1.1，只读不 fork）
	Courier struct {
		BaseURL string `json:",default=http://localhost:8080,env=COURIER_GATEWAY_URL"`
		Timeout int64  `json:",default=5000,env=COURIER_TIMEOUT"`
	}

	// GameCatalog 角色权威（/me）：商品配置等管理面按平台角色放行
	GameCatalog struct {
		BaseURL string `json:",default=http://localhost:8890,env=GAMECATALOG_BASE_URL"`
		Timeout int64  `json:",default=5000,env=GAMECATALOG_TIMEOUT"`
	}

	// Channel 渠道适配：Sandbox 为开发/沙箱专用（HMAC-SHA256 回调），
	// 真实渠道需实现 ChannelAdapter 并以环境变量注入密钥，不得用沙箱 HMAC
	// 冒充支付机构验签（payment.md v1.1 安全边界 2）。
	Channel struct {
		SandboxSecret string `json:",env=PAYMENTS_SANDBOX_SECRET"`
	}
}
