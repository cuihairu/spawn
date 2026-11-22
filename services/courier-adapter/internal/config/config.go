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

	// Payments 订单事实权威（GP-307 售后工单 orderId 归属强校验）：经
	// GET /internal/orders/:orderId（X-Internal-Token）核验订单属于当前
	// 用户/游戏。Token 未配置或上游不可达 fail-open 保留卫生透传（校验为
	// 增强事实，不阻断提单）；订单明确不属于本人/本游戏 → 400。
	Payments struct {
		BaseURL       string `json:",default=http://localhost:8898,env=PAYMENTS_URL"`
		InternalToken string `json:",env=INTERNAL_TOKEN,optional"`
		Timeout       int64  `json:",default=3000,env=PAYMENTS_TIMEOUT"`
	}

	// Ticket 启动票据 TTL（秒）：短期、一次性，重启即失效（不落盘）
	Ticket struct {
		TTLSeconds int64 `json:",default=120"`
	}

	// Promotions GP-303 回流判定源上报（游戏会话签发成功 = 账号×游戏活跃
	// 事实 → promotions /internal/player-activity）。BaseURL 空 = 未接促销
	// 服务（no-op，上报不发起，游戏会话主流程不受影响）；Token 与
	// promotions 侧 INTERNAL_TOKEN 同值。
	Promotions struct {
		BaseURL               string `json:",env=PROMOTIONS_URL,optional"`
		InternalToken         string `json:",env=INTERNAL_TOKEN,optional"`
		Timeout               int64  `json:",default=3000,env=PROMOTIONS_TIMEOUT"`
		OutboxIntervalSeconds int64  `json:",default=30,env=PROMOTIONS_OUTBOX_INTERVAL_SECONDS"`
		OutboxMaxAttempts     int64  `json:",default=10,env=PROMOTIONS_OUTBOX_MAX_ATTEMPTS"`
		OutboxRetentionHours  int64  `json:",default=168,env=PROMOTIONS_OUTBOX_RETENTION_HOURS"`
	}
}
