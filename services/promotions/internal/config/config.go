package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	// 数据库配置（DSN 含 file:/.db 走 SQLite，否则 MySQL，与 payments 同款探测）
	MySQL struct {
		DataSource string `json:",env=DATASOURCE"`
	}

	// 平台 JWT 校验密钥（与 user-service 共享）
	Auth struct {
		JWTSecret string `json:",env=JWT_SECRET"`
	}

	// GameCatalog 角色权威（/me）：活动配置管理面按平台角色放行
	GameCatalog struct {
		BaseURL string `json:",default=http://localhost:8890,env=GAMECATALOG_BASE_URL"`
		Timeout int64  `json:",default=5000,env=GAMECATALOG_TIMEOUT"`
	}

	// GP-302 奖励评估 S2S 密钥（与 payments 共享；未配置 fail-closed 403）
	Reward struct {
		Secret string `json:",env=PROMOTIONS_REWARD_SECRET,optional"`
	}

	// GP-306 活动消息触发：受众拉自 game-catalog 预约人群，投递走 community
	// 站内通知（realtime-hub 远程推送在 community 侧）；CommunityUrl 空 = 不
	// 投递，CatalogUrl 空 = 不拉受众（活动发布本身不受影响，本地开发零依赖）；
	// Token 须与两侧 INTERNAL_TOKEN 一致。
	Notify struct {
		CommunityUrl string `json:",env=COMMUNITY_URL,optional"`
		CatalogUrl   string `json:",env=GAMECATALOG_BASE_URL,optional"`
		Token        string `json:",env=INTERNAL_TOKEN,optional"`
	}

	// GP-402 经营分析事实投递（F24）：reward_granted 事实事件 → data-panel
	// /internal/events（X-Internal-Token）。BaseURL 未配置 = no-op，
	// 部署面未带数据面板时行为不变；fire-and-forget 不影响发放主流程。
	Analytics struct {
		BaseURL string `json:",env=DATA_PANEL_URL,optional"`
		Token   string `json:",env=DATA_PANEL_INTERNAL_TOKEN,optional"`
		Timeout int64  `json:",default=3000,env=DATA_PANEL_TIMEOUT"`
	}
}
