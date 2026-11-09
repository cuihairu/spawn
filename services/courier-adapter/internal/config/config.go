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

	// Courier 网关地址（契约 Frozen：scope v1 / auth v1.1，只读不 fork）
	Courier struct {
		BaseURL string `json:",default=http://localhost:8080,env=COURIER_GATEWAY_URL"`
		Timeout int64  `json:",default=5000,env=COURIER_TIMEOUT"`
	}

	// Credential 绑定凭据加密密钥：代登录随机密码以 AES-GCM 加密落库，
	// 不存明文（接入设计 §1.3）。生产必须显式覆盖。
	Credential struct {
		Key string `json:",env=CREDENTIAL_KEY"`
	}

	// GameCatalog 角色权威（/me）：映射管理等管理面按平台角色放行
	GameCatalog struct {
		BaseURL string `json:",default=http://localhost:8890,env=GAMECATALOG_BASE_URL"`
		Timeout int64  `json:",default=5000,env=GAMECATALOG_TIMEOUT"`
	}

	// UserService 平台用户资料（取绑定邮箱用，邮箱仅本人可见所以透传本人令牌）
	UserService struct {
		BaseURL string `json:",default=http://localhost:8888,env=USER_SERVICE_URL"`
		Timeout int64  `json:",default=5000,env=USER_SERVICE_TIMEOUT"`
	}

	// Ticket 启动票据 TTL（秒）：短期、一次性，重启即失效（不落盘）
	Ticket struct {
		TTLSeconds int64 `json:",default=120"`
	}
}
