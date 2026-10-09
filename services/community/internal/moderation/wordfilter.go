// Package moderation 承载内容审核的敏感词过滤（阻塞式）：词表命中即拒绝写入。
// 词表来源：配置 Moderation.BlockedWords（空则用内置默认表）；匹配为大小写
// 不敏感的子串包含，命中返回首个违规词供调用方提示用户。
package moderation

import (
	"strings"
)

// DefaultBlockedWords 内置默认敏感词表（演示级最小集；生产经配置覆盖）。
// 选词避开种子数据与正常演示文案，保证开箱即用不误伤。
var DefaultBlockedWords = []string{"赌博", "诈骗", "外挂", "代练"}

// NormalizeWords 词表归一化：去空白、小写、去重（保序）。
func NormalizeWords(words []string) []string {
	seen := make(map[string]bool, len(words))
	out := make([]string, 0, len(words))
	for _, w := range words {
		n := strings.ToLower(strings.TrimSpace(w))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// FirstHit 返回文本命中的首个敏感词（大小写不敏感）；未命中 ok=false。
func FirstHit(words []string, text string) (string, bool) {
	lower := strings.ToLower(text)
	for _, w := range words {
		if w == "" {
			continue
		}
		if strings.Contains(lower, w) {
			return w, true
		}
	}
	return "", false
}
