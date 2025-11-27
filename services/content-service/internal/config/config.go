// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	// DataSource 配置攻略和评论数据文件
	DataSource struct {
		GuidesFile   string `json:",default=data/guides.json"`
		CommentsFile string `json:",default=data/comments.json"`
	}
}
