// Package analytics 承载 GP-402 经营分析查询域的 logic（漏斗/留存/
// 活动效果/账单核对）与 S2S 事件摄入。查询一律要求 AdminUserIds 名单
// 权限（svc.IsAnalyticsAdmin），摄入要求 X-Internal-Token 严格比对。
package analytics

import (
	"time"

	"github.com/tappi/tappi/services/data-panel/internal/httperr"
)

// normalizeBounds 归一查询窗口：from/to 支持 RFC3339 或 10 位日期
// （日期按全天边界展开）；缺省 from=30 天前、to=当前。返回 (
// 归一 from, 归一 to, 错误)。日期比较走字符串（occurred_at 为 RFC3339
// 定宽，字典序即时间序），两侧同格式即可正确比较。
func normalizeBounds(from, to string) (string, string, error) {
	if from == "" {
		from = time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339)
	}
	if to == "" {
		to = time.Now().UTC().Format(time.RFC3339)
	}
	if len(from) == 10 {
		from += "T00:00:00Z"
	}
	if len(to) == 10 {
		to += "T23:59:59Z"
	}
	// 仅接受能解析的时间表达（防脏查询参数把字符串比较带偏）
	f, errF := time.Parse(time.RFC3339, from)
	t, errT := time.Parse(time.RFC3339, to)
	if errF != nil || errT != nil {
		return "", "", httperr.BadRequest("from/to must be RFC3339 or YYYY-MM-DD")
	}
	if f.After(t) {
		return "", "", httperr.BadRequest("from must not be after to")
	}
	return from, to, nil
}
