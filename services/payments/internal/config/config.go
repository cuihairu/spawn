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

	// Delivery 发货接缝（GP-203）：teller Deliver 钩子（进程内）的生产落地
	// 面为签名 webhook。Secret 未配置 = 分发 fail-closed 全部 403（与
	// teller 空 ChannelSecret 同语义），生产必须注入。
	Delivery struct {
		Secret               string `json:",env=PAYMENTS_DELIVERY_SECRET"`
		RetryIntervalSeconds int64  `json:",default=60,env=PAYMENTS_DELIVERY_RETRY_SECONDS"`
		TimeoutMs            int64  `json:",default=5000,env=PAYMENTS_DELIVERY_TIMEOUT"`
	}

	// Promotions 活动/促销服务（GP-302）：订单 PAID 后推送订单事实做倍率
	// 赠送评估。BaseURL 未配置 = 不入队不分发（部署面未带促销服务时行为
	// 不变）；Secret 与 promotions 侧 PROMOTIONS_REWARD_SECRET 同值。
	Promotions struct {
		BaseURL  string `json:",env=PROMOTIONS_URL,optional"`
		Secret   string `json:",env=PROMOTIONS_REWARD_SECRET,optional"`
		Timeout  int64  `json:",default=5000,env=PROMOTIONS_TIMEOUT"`
		Interval int64  `json:",default=60,env=PROMOTIONS_EVAL_INTERVAL_SECONDS"`
	}
}
