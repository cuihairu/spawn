package logic

import (
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"
)

// --- format 归一化助手 ---

func TestNormalizeGuideFormat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"", "text"},
		{"text", "text"},
		{"markdown", "markdown"},
		{"html", ""},
		{"Markdown", ""},
		{" md ", ""},
	}
	for _, c := range cases {
		if got := normalizeGuideFormat(c.in); got != c.want {
			t.Errorf("normalizeGuideFormat(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- 创建攻略：format 校验与持久化 ---

func TestCreateGuide_FormatValidation(t *testing.T) {
	t.Parallel()

	t.Run("empty format defaults to text", func(t *testing.T) {
		l := NewCreateGuideLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
		resp, err := l.CreateGuide(&types.CreateGuideRequest{GameId: "g1", Title: "t", Content: "c"})
		if err != nil || resp.Code != http.StatusOK {
			t.Fatalf("resp=%+v err=%v, want 200", resp, err)
		}
		if resp.Data.Format != "text" {
			t.Fatalf("format = %q, want text", resp.Data.Format)
		}
	})

	t.Run("explicit text format stored", func(t *testing.T) {
		l := NewCreateGuideLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
		resp, err := l.CreateGuide(&types.CreateGuideRequest{GameId: "g1", Title: "t", Content: "c", Format: "text"})
		if err != nil || resp.Code != http.StatusOK {
			t.Fatalf("resp=%+v err=%v, want 200", resp, err)
		}
		if resp.Data.Format != "text" {
			t.Fatalf("format = %q, want text", resp.Data.Format)
		}
	})

	t.Run("markdown format stored", func(t *testing.T) {
		l := NewCreateGuideLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
		resp, err := l.CreateGuide(&types.CreateGuideRequest{GameId: "g1", Title: "t", Content: "c", Format: "markdown"})
		if err != nil || resp.Code != http.StatusOK {
			t.Fatalf("resp=%+v err=%v, want 200", resp, err)
		}
		if resp.Data.Format != "markdown" {
			t.Fatalf("format = %q, want markdown", resp.Data.Format)
		}
	})

	t.Run("invalid format rejected", func(t *testing.T) {
		l := NewCreateGuideLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
		resp, err := l.CreateGuide(&types.CreateGuideRequest{GameId: "g1", Title: "t", Content: "c", Format: "html"})
		if err != nil || resp.Code != http.StatusBadRequest || resp.Message != "内容格式仅支持 text 或 markdown" {
			t.Fatalf("resp=%+v err=%v, want 400 内容格式仅支持 text 或 markdown", resp, err)
		}
	})
}

// --- 更新攻略：format 校验与写入 ---

func TestUpdateGuide_FormatValidation(t *testing.T) {
	t.Parallel()

	t.Run("invalid format rejected", func(t *testing.T) {
		l := NewUpdateGuideLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
		resp, err := l.UpdateGuide(&types.UpdateGuideRequest{Id: 1, Format: "html"})
		if err != nil || resp.Code != http.StatusBadRequest || resp.Message != "内容格式仅支持 text 或 markdown" {
			t.Fatalf("resp=%+v err=%v, want 400 内容格式仅支持 text 或 markdown", resp, err)
		}
	})

	t.Run("markdown format applied", func(t *testing.T) {
		l := NewUpdateGuideLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
		resp, err := l.UpdateGuide(&types.UpdateGuideRequest{Id: 1, Format: "markdown"})
		if err != nil || resp.Code != http.StatusOK {
			t.Fatalf("resp=%+v err=%v, want 200", resp, err)
		}
		if resp.Data.Format != "markdown" {
			t.Fatalf("format = %q, want markdown", resp.Data.Format)
		}
	})

	t.Run("empty format keeps stored value", func(t *testing.T) {
		l := NewUpdateGuideLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
		resp, err := l.UpdateGuide(&types.UpdateGuideRequest{Id: 1, Title: "renamed"})
		if err != nil || resp.Code != http.StatusOK {
			t.Fatalf("resp=%+v err=%v, want 200", resp, err)
		}
		if resp.Data.Format != "text" {
			t.Fatalf("format = %q, want text (unchanged)", resp.Data.Format)
		}
	})
}

// --- mapper：format 字段往返映射 ---

func TestModelGuideToType_FormatMapped(t *testing.T) {
	t.Parallel()

	m := &model.Guide{Id: 1, Format: "markdown"}
	if got := modelGuideToType(m).Format; got != "markdown" {
		t.Fatalf("modelGuideToType format = %q, want markdown", got)
	}
}
