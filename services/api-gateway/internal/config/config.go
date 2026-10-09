// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	// Auth 上传入口的 JWT 本地校验：secret 与 user-service 的 Auth.JWTSecret
	// 保持一致（HS256），claims 口径见 internal/upload.UserClaims。
	Auth struct {
		JWTSecret string `json:",env=JWT_SECRET"`
	}

	// Upload 统一上传入口：图片落本地盘（方案 A），业务服务只存返回的 URL。
	Upload struct {
		Dir      string `json:",default=./uploads"`
		MaxBytes int64  `json:",default=10485760"`
	}

	Upstreams struct {
		UserService struct {
			BaseURL string `json:",env=USER_SERVICE_URL"`
			Timeout int64  `json:",default=5000"`
		}
		GameCatalog struct {
			BaseURL string `json:",env=GAME_SERVICE_URL"`
			Timeout int64  `json:",default=5000"`
		}
		Content struct {
			BaseURL string `json:",env=CONTENT_SERVICE_URL"`
			Timeout int64  `json:",default=5000"`
		}
		Community struct {
			BaseURL string `json:",env=COMMUNITY_SERVICE_URL"`
			Timeout int64  `json:",default=5000"`
		}
		DataPanel struct {
			BaseURL string `json:",env=DATA_PANEL_URL"`
			Timeout int64  `json:",default=5000"`
		}
	}
}
