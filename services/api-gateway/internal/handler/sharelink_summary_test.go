package handler

import (
	"strings"
	"testing"
)

// summary 的超长截断分支：分享卡描述按 rune 截到 120 + 省略号，
// 不劈开多字节字符；短文本原样返回。
func TestShareCardSummary(t *testing.T) {
	if got := summary("  短正文  "); got != "短正文" {
		t.Fatalf("short summary = %q, want trimmed", got)
	}

	long := strings.Repeat("帖", 150)
	got := []rune(summary(long))
	if len(got) != 121 || string(got[120]) != "…" {
		t.Fatalf("long summary = %d runes, want 121 ending with ellipsis", len(got))
	}
}
