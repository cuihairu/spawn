// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	DataSource struct {
		TopicsFile  string `json:",default=data/topics.json"`
		PostsFile   string `json:",default=data/posts.json"`
		FollowsFile string `json:",default=data/follows.json"`
	}

	Auth struct {
		JWTSecret string `json:",env=JWT_SECRET"`
	}
}
