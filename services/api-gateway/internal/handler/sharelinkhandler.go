// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package handler

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// shareCardPage 分享卡跳板页模板：og 卡片 meta + 深链（spawn:///post/:id）
// 与 Web 入口。相对链接（og:url / web 按钮）按网关部署域解析。
const shareCardPage = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Description}}">
<meta property="og:type" content="article">
<meta property="og:url" content="/s/p/{{.Id}}">
<title>{{.Title}}</title>
<style>
  body { font-family: -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif;
         background: #14161a; color: #e8e8e8; margin: 0; display: flex;
         align-items: center; justify-content: center; min-height: 100vh; }
  .card { max-width: 420px; width: 88%; background: #1e2126; border: 1px solid #333;
          border-radius: 14px; padding: 24px; }
  h1 { font-size: 18px; margin: 12px 0 8px; line-height: 1.4; }
  p  { font-size: 13px; color: #9aa1ab; margin: 0 0 18px; line-height: 1.6; }
  a  { display: block; text-align: center; padding: 12px; border-radius: 10px;
       text-decoration: none; font-size: 14px; margin-bottom: 10px; }
  .app  { background: #f2c14e; color: #1a1105; font-weight: 600; }
  .web  { border: 1px solid #444; color: #e8e8e8; }
  .tag { color: #f2c14e; font-size: 12px; }
</style>
</head>
<body>
<div class="card">
  <div class="tag">spawn 社区</div>
  <h1>{{.Title}}</h1>
  <p>{{.Description}}</p>
  <a class="app" href="{{.AppURL}}">在 spawn 中打开</a>
  <a class="web" href="{{.WebURL}}">在浏览器中查看</a>
</div>
</body>
</html>`

type shareCardData struct {
	Title       string
	Description string
	Id          string
	// AppURL 是 spawn:// 自定义 scheme：html/template 默认只放行
	// http(s)/mailto 等已知 scheme（其余输出 #ZgotmplZ），这里声明为
	// template.URL 明确信任该值（拼接逻辑见下，仅由本 handler 生成）。
	AppURL template.URL
	WebURL string
}

// summary 截断帖子正文做分享卡摘要（按 rune 截断避免劈开 CJK）。
func summary(content string) string {
	const maxRunes = 120
	runes := []rune(strings.TrimSpace(content))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}

func ShareLinkHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	tmpl := template.Must(template.New("shareCard").Parse(shareCardPage))
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Id int64 `path:"id"`
		}
		if err := httpx.Parse(r, &req); err != nil || req.Id <= 0 {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}

		// 拉帖子详情铺满卡片；失败（下架/上游抖动）仍输出通用卡片，
		// 深链入口不因详情不可达而失效。
		post, err := svcCtx.Community.GetPost(r.Context(), req.Id)
		data := shareCardData{
			Title:  "在 spawn 里看看这篇帖子",
			Id:     strconv.FormatInt(req.Id, 10),
			AppURL: template.URL("spawn:///post/" + strconv.FormatInt(req.Id, 10)),
			WebURL: "/community/posts/" + strconv.FormatInt(req.Id, 10),
		}
		if err == nil && post != nil {
			data.Title = post.Title
			data.Description = summary(post.Content)
		} else {
			data.Description = "game 社区帖子分享：打开链接回到分享的帖子。"
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if err := tmpl.Execute(w, data); err != nil {
			// 模板执行失败近乎不可能；兜底纯文本不吞错误
			http.Error(w, "render failed", http.StatusInternalServerError)
		}
	}
}
