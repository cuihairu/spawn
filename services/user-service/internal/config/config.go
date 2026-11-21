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
			// InternalToken GP-403 渠道归因 S2S（game-catalog
			// /internal/promo/clicks/:id/resolve 的 X-Internal-Token）；
			// 空 = 点击解析 403，注册按自然流量处理（归因不阻断注册）。
			InternalToken string `json:",env=GAMECATALOG_INTERNAL_TOKEN,optional"`
		}
	}

	// GP-402 经营分析事实投递（F24）：register/login 事实事件 → data-panel
	// /internal/events。BaseURL 未配置 = no-op（数据面板未部署时行为不变）。
	Analytics struct {
		BaseURL               string `json:",env=DATA_PANEL_URL,optional"`
		Token                 string `json:",env=DATA_PANEL_INTERNAL_TOKEN,optional"`
		Timeout               int64  `json:",default=3000,env=DATA_PANEL_TIMEOUT"`
		OutboxIntervalSeconds int64  `json:",default=30,env=DATA_PANEL_OUTBOX_INTERVAL_SECONDS"`
		OutboxMaxAttempts     int64  `json:",default=10,env=DATA_PANEL_OUTBOX_MAX_ATTEMPTS"`
		OutboxRetentionHours  int64  `json:",default=168,env=DATA_PANEL_OUTBOX_RETENTION_HOURS"`
	}

	// GP-403 渠道归因补偿：注册携带 click_id 时向 game-catalog 解析渠道归因
	//（S2S /internal/promo/clicks/:id/resolve）。此前同步 HTTP 失败即丢归因
	//（game-catalog 短时不可达），归因意图先落 outbox（幂等），失败留 pending
	// 由 Relay 重投；重投锚定原始 registered_at 判定、回写 user_id 幂等
	//（一点击一归因）。Outbox* 为本地补偿重投节奏。
	PromoAttr struct {
		// OutboxIntervalSeconds 重投扫描周期（默认 30s）。
		OutboxIntervalSeconds int64 `json:",default=30,env=PROMO_ATTR_OUTBOX_INTERVAL_SECONDS"`
		// OutboxMaxAttempts 重试上限（默认 10，超出置 dropped 留痕）。
		OutboxMaxAttempts int64 `json:",default=10,env=PROMO_ATTR_OUTBOX_MAX_ATTEMPTS"`
		// OutboxRetentionHours 已终态行保留窗口（默认 168h=7 天）。
		OutboxRetentionHours int64 `json:",default=168,env=PROMO_ATTR_OUTBOX_RETENTION_HOURS"`
	}
}
