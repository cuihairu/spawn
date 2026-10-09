package moderation

import "testing"

func TestFirstHit(t *testing.T) {
	words := NormalizeWords([]string{"赌博", "外挂", "赌博", "  ", "外挂"})

	cases := []struct {
		text string
		want string
		ok   bool
	}{
		{"正常帖子正文", "", false},
		{"这个外挂真好用", "外挂", true},
		{"讨论赌博网站", "赌博", true},
		{"大小写混合：WAI挂", "", false}, // 中文无大小写；英文词表才涉及
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := FirstHit(words, tc.text)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("FirstHit(%q) = %q,%v, want %q,%v", tc.text, got, ok, tc.want, tc.ok)
		}
	}
}

func TestFirstHitCaseInsensitive(t *testing.T) {
	words := NormalizeWords([]string{"spam"})
	if got, ok := FirstHit(words, "SPAM message"); !ok || got != "spam" {
		t.Fatalf("FirstHit = %q,%v, want spam,true", got, ok)
	}
}

func TestNormalizeWords(t *testing.T) {
	got := NormalizeWords([]string{" B ", "a", "b", "a", ""})
	if len(got) != 2 || got[0] != "b" || got[1] != "a" {
		t.Fatalf("NormalizeWords = %v, want [b a]", got)
	}
}
