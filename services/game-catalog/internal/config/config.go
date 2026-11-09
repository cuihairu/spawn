// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	// MySQL 游戏目录数据库。DSN 含 file: 或 .db 时走 SQLite（开发/测试），
	// 否则按 MySQL 连接（生产）；默认本地 SQLite 文件，零配置可跑。
	MySQL struct {
		DataSource string `json:",env=DATASOURCE,default=file:data/games.db"`
	}

	// Package 包体存储（GP-105 受控下载）：本地磁盘目录托管合作方上传的
	// 渠道包，按版本 id 组织，仅供已发布版本受控分发；生产部署须挂持久卷
	// 并纳入备份（接入设计 §4）。留空时包体上传/下载端点返回 500（未配置）。
	Package struct {
		Dir string `json:",env=PACKAGE_DIR,default=data/packages"`
	}

	// Auth JWT 校验配置（与 user-service 共享密钥，令牌由 user-service 签发）
	Auth struct {
		JWTSecret string `json:",env=JWT_SECRET"`

		// AdminUserIds 引导管理员账号（逗号分隔的 user-service 用户 id），
		// 启动时写入 platform_roles 作为 platform_admin（幂等；首个管理员引导）。
		AdminUserIds []string `json:",env=ADMIN_USER_IDS,optional"`
	}

	// GP-306 开服提醒/版本通知：经 community /internal/notifications 扇出
	//（站内信 + realtime-hub 远程推送）。Url 空则 no-op（通知不落库不推送，
	// 主操作不受影响）；Token 须与 community INTERNAL_TOKEN 一致。
	Notify struct {
		Url   string `json:",env=COMMUNITY_URL,optional"`
		Token string `json:",env=INTERNAL_TOKEN,optional"`
	}

	// Internal 本服务 S2S 端点守卫（/internal/games/:id/reservation-users，
	// 供 promotions 拉活动通知受众）；Token 空 = 恒 403（fail-closed）。
	Internal struct {
		Token string `json:",env=INTERNAL_TOKEN,optional"`
	}
}
