package logic

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"
	"github.com/tappi/tappi/services/content-service/utils"

	_ "github.com/mattn/go-sqlite3"
)

// newFavoriteSvcCtx 建含收藏仓储的测试上下文：一条已发布攻略（id=1）+
// 一条草稿（id=2），覆盖收藏目标的可见性分支。
func newFavoriteSvcCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()

	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	guideModel := model.NewGuideModel(db)
	if err := guideModel.CreateGuidesTable(); err != nil {
		t.Fatalf("create guides table: %v", err)
	}
	if err := guideModel.Seed([]*model.Guide{
		{Id: 1, GameId: "g1", GameTitle: "G1", Title: "pub", Content: "c", AuthorId: 1,
			AuthorName: "u1", IsPublished: true,
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		{Id: 2, GameId: "g1", GameTitle: "G1", Title: "draft", Content: "c", AuthorId: 1,
			AuthorName: "u1", IsPublished: false,
			CreatedAt: "2024-01-02T00:00:00Z", UpdatedAt: "2024-01-02T00:00:00Z"},
	}); err != nil {
		t.Fatalf("seed guides: %v", err)
	}

	favoriteModel := model.NewFavoriteModel(db)
	if err := favoriteModel.CreateFavoritesTable(); err != nil {
		t.Fatalf("create favorites table: %v", err)
	}

	return &svc.ServiceContext{
		GuideRepository:    guideModel,
		CommentRepository:  model.NewCommentModel(db),
		FavoriteRepository: favoriteModel,
		Auth:               utils.NewAuth("favorite-secret"),
	}
}

// --- FavoriteGuide ---

func TestFavoriteGuide_Unauthorized(t *testing.T) {
	l := NewFavoriteGuideLogic(context.Background(), newFavoriteSvcCtx(t))
	resp, err := l.FavoriteGuide(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusUnauthorized || resp.Message != "请先登录" {
		t.Fatalf("resp=%+v err=%v, want 401 请先登录", resp, err)
	}
}

func TestFavoriteGuide_GuideNotFound(t *testing.T) {
	l := NewFavoriteGuideLogic(userCtx(2, "u2"), newFavoriteSvcCtx(t))
	resp, err := l.FavoriteGuide(&types.FavoriteGuideRequest{Id: 99})
	if err != nil || resp.Code != http.StatusNotFound {
		t.Fatalf("resp=%+v err=%v, want 404", resp, err)
	}
}

func TestFavoriteGuide_DraftTreatedAsMissing(t *testing.T) {
	l := NewFavoriteGuideLogic(userCtx(2, "u2"), newFavoriteSvcCtx(t))
	// 与点赞同语义：未发布攻略对外不存在
	resp, err := l.FavoriteGuide(&types.FavoriteGuideRequest{Id: 2})
	if err != nil || resp.Code != http.StatusNotFound {
		t.Fatalf("resp=%+v err=%v, want 404", resp, err)
	}
}

func TestFavoriteGuide_AddAndCount(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)

	l := NewFavoriteGuideLogic(userCtx(2, "u2"), svcCtx)
	resp, err := l.FavoriteGuide(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusOK || !resp.Favorited || resp.Count != 1 {
		t.Fatalf("resp=%+v err=%v, want 200 favorited count=1", resp, err)
	}

	// 幂等：重复收藏计数不涨
	if resp, err := l.FavoriteGuide(&types.FavoriteGuideRequest{Id: 1}); err != nil ||
		resp.Code != http.StatusOK || resp.Count != 1 {
		t.Fatalf("re-favorite resp=%+v err=%v, want 200 count=1", resp, err)
	}

	// 独立用户计数累加
	other := NewFavoriteGuideLogic(userCtx(3, "u3"), svcCtx)
	if resp, err := other.FavoriteGuide(&types.FavoriteGuideRequest{Id: 1}); err != nil ||
		resp.Count != 2 {
		t.Fatalf("second user resp=%+v err=%v, want count=2", resp, err)
	}
}

// --- UnfavoriteGuide ---

func TestUnfavoriteGuide_Unauthorized(t *testing.T) {
	l := NewUnfavoriteGuideLogic(context.Background(), newFavoriteSvcCtx(t))
	resp, err := l.UnfavoriteGuide(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusUnauthorized {
		t.Fatalf("resp=%+v err=%v, want 401", resp, err)
	}
}

func TestUnfavoriteGuide_Idempotent(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)

	add := NewFavoriteGuideLogic(userCtx(2, "u2"), svcCtx)
	if _, err := add.FavoriteGuide(&types.FavoriteGuideRequest{Id: 1}); err != nil {
		t.Fatalf("favorite: %v", err)
	}

	remove := NewUnfavoriteGuideLogic(userCtx(2, "u2"), svcCtx)
	resp, err := remove.UnfavoriteGuide(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusOK || resp.Favorited || resp.Count != 0 {
		t.Fatalf("resp=%+v err=%v, want 200 unfavorited count=0", resp, err)
	}
	// 再删一次同样成功（幂等）
	if resp, err := remove.UnfavoriteGuide(&types.FavoriteGuideRequest{Id: 1}); err != nil ||
		resp.Code != http.StatusOK {
		t.Fatalf("re-remove resp=%+v err=%v, want 200", resp, err)
	}
}

func TestUnfavoriteGuide_GuideNotFound(t *testing.T) {
	l := NewUnfavoriteGuideLogic(userCtx(2, "u2"), newFavoriteSvcCtx(t))
	resp, err := l.UnfavoriteGuide(&types.FavoriteGuideRequest{Id: 99})
	if err != nil || resp.Code != http.StatusNotFound {
		t.Fatalf("resp=%+v err=%v, want 404", resp, err)
	}
}

// --- GetFavoriteStatus ---

func TestGetFavoriteStatus_AnonymousCountOnly(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)

	add := NewFavoriteGuideLogic(userCtx(2, "u2"), svcCtx)
	if _, err := add.FavoriteGuide(&types.FavoriteGuideRequest{Id: 1}); err != nil {
		t.Fatalf("favorite: %v", err)
	}

	l := NewGetFavoriteStatusLogic(context.Background(), svcCtx)
	resp, err := l.GetFavoriteStatus(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusOK || resp.Favorited || resp.Count != 1 {
		t.Fatalf("resp=%+v err=%v, want 200 anonymous favorited=false count=1", resp, err)
	}
}

func TestGetFavoriteStatus_AuthenticatedTrue(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)

	add := NewFavoriteGuideLogic(userCtx(2, "u2"), svcCtx)
	if _, err := add.FavoriteGuide(&types.FavoriteGuideRequest{Id: 1}); err != nil {
		t.Fatalf("favorite: %v", err)
	}

	l := NewGetFavoriteStatusLogic(userCtx(2, "u2"), svcCtx)
	resp, err := l.GetFavoriteStatus(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusOK || !resp.Favorited || resp.Count != 1 {
		t.Fatalf("resp=%+v err=%v, want 200 favorited count=1", resp, err)
	}
}

// --- ListFavorites ---

func TestListFavorites_Unauthorized(t *testing.T) {
	l := NewListFavoritesLogic(context.Background(), newFavoriteSvcCtx(t))
	resp, err := l.ListFavorites(&types.ListFavoritesRequest{})
	if err != nil || resp.Code != http.StatusUnauthorized || resp.Message != "请先登录" {
		t.Fatalf("resp=%+v err=%v, want 401 请先登录", resp, err)
	}
}

func TestListFavorites_OnlyOwnFavorites(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)

	u2 := NewFavoriteGuideLogic(userCtx(2, "u2"), svcCtx)
	u3 := NewFavoriteGuideLogic(userCtx(3, "u3"), svcCtx)
	if _, err := u2.FavoriteGuide(&types.FavoriteGuideRequest{Id: 1}); err != nil {
		t.Fatalf("u2 favorite: %v", err)
	}
	if _, err := u3.FavoriteGuide(&types.FavoriteGuideRequest{Id: 1}); err != nil {
		t.Fatalf("u3 favorite: %v", err)
	}

	l := NewListFavoritesLogic(userCtx(2, "u2"), svcCtx)
	resp, err := l.ListFavorites(&types.ListFavoritesRequest{})
	if err != nil || resp.Code != http.StatusOK {
		t.Fatalf("resp=%+v err=%v, want 200", resp, err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 || resp.Data[0].Id != 1 || resp.Page != 1 {
		t.Fatalf("total=%d len=%d page=%d, want 1/1/1", resp.Total, len(resp.Data), resp.Page)
	}
}

func TestListFavorites_DefaultsAndPagination(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)

	// 请求体缺省 page/page_size 时钳制为 1/20
	resp, err := NewListFavoritesLogic(userCtx(2, "u2"), svcCtx).
		ListFavorites(&types.ListFavoritesRequest{})
	if err != nil || resp.Code != http.StatusOK {
		t.Fatalf("empty user resp=%+v err=%v, want 200", resp, err)
	}
	if resp.Total != 0 || len(resp.Data) != 0 {
		t.Fatalf("total=%d len=%d, want 0/0", resp.Total, len(resp.Data))
	}
}
