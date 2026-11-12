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

	// Internal S2S 摄入口令（GP-402）：/internal/events 事件摄入按
	// X-Internal-Token 严格比对，空令牌（含未配置）时摄入端点整体
	// 拒绝（fail-closed，不留默认口令）。
	Internal struct {
		Token string `json:",env=INTERNAL_TOKEN,optional"`
	}

	// Analytics 经营分析查询权限（GP-402）：漏斗/留存/活动效果/账单核对
	// 仅对 AdminUserIds 名单内的用户开放（社区 Moderation.AdminUserIds 同款）。
	Analytics struct {
		AdminUserIds []string `json:",optional"`
	}
}
