package handler

// 覆盖 handler 逻辑错误分支中的「存储层失败 → 500」臂（httpx.ErrorCtx）。
// 通过替换 svc.ServiceContext 的仓储实现注入失败，请求经真实路由打到
// handler 的 ErrorCtx。与 TestRoutes_LogicErrors（400/404 校验错误）互补。

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// failingAdminStore 内嵌真实 AdminStore，按字段注入单方法失败；其余委托。
type failingAdminStore struct {
	model.AdminStore
	listPartnersErr   error
	listReviewsErr    error
	listVersionsErr   error
	getRoleErr        error
	listAuditErr      error
	listReportsErr    error
	listFavoritesErr  error
	removeFavoriteErr error
	listPromoErr      error
	channelStatsErr   error
	getPromoClickErr  error
	listMyReservErr   error
	listResUsersErr   error
}

var _ model.AdminStore = (*failingAdminStore)(nil)

func (s *failingAdminStore) ListPartners() ([]*model.Partner, error) {
	if s.listPartnersErr != nil {
		return nil, s.listPartnersErr
	}
	return s.AdminStore.ListPartners()
}

func (s *failingAdminStore) ListReviews(status string, limit, offset int) ([]*model.GameReview, int, error) {
	if s.listReviewsErr != nil {
		return nil, 0, s.listReviewsErr
	}
	return s.AdminStore.ListReviews(status, limit, offset)
}

func (s *failingAdminStore) ListVersionsByGame(gameId string) ([]*model.GameVersion, error) {
	if s.listVersionsErr != nil {
		return nil, s.listVersionsErr
	}
	return s.AdminStore.ListVersionsByGame(gameId)
}

func (s *failingAdminStore) GetRoleWithPartner(userId int64) (string, int64, error) {
	if s.getRoleErr != nil {
		return "", 0, s.getRoleErr
	}
	return s.AdminStore.GetRoleWithPartner(userId)
}

func (s *failingAdminStore) ListAuditLogs(resourceType string, limit, offset int) ([]*model.AuditLog, int, error) {
	if s.listAuditErr != nil {
		return nil, 0, s.listAuditErr
	}
	return s.AdminStore.ListAuditLogs(resourceType, limit, offset)
}

func (s *failingAdminStore) ListReviewReports(status string, limit, offset int) ([]*model.ReviewReport, int, error) {
	if s.listReportsErr != nil {
		return nil, 0, s.listReportsErr
	}
	return s.AdminStore.ListReviewReports(status, limit, offset)
}

func (s *failingAdminStore) ListMyFavorites(userId int64, limit int) ([]*model.GameFavorite, error) {
	if s.listFavoritesErr != nil {
		return nil, s.listFavoritesErr
	}
	return s.AdminStore.ListMyFavorites(userId, limit)
}

func (s *failingAdminStore) RemoveFavorite(gameId string, userId int64) error {
	if s.removeFavoriteErr != nil {
		return s.removeFavoriteErr
	}
	return s.AdminStore.RemoveFavorite(gameId, userId)
}

func (s *failingAdminStore) ListPromoChannels(limit, offset int) ([]*model.PromoChannel, error) {
	if s.listPromoErr != nil {
		return nil, s.listPromoErr
	}
	return s.AdminStore.ListPromoChannels(limit, offset)
}

func (s *failingAdminStore) ChannelStats(channelCode, from, to string) (*model.ChannelStats, error) {
	if s.channelStatsErr != nil {
		return nil, s.channelStatsErr
	}
	return s.AdminStore.ChannelStats(channelCode, from, to)
}

func (s *failingAdminStore) GetPromoClick(clickId string) (*model.PromoClick, error) {
	if s.getPromoClickErr != nil {
		return nil, s.getPromoClickErr
	}
	return s.AdminStore.GetPromoClick(clickId)
}

func (s *failingAdminStore) ListMyReservations(userId int64, limit int) ([]*model.ServerReservation, error) {
	if s.listMyReservErr != nil {
		return nil, s.listMyReservErr
	}
	return s.AdminStore.ListMyReservations(userId, limit)
}

func (s *failingAdminStore) ListReservationUsersByGame(gameId string, limit int) ([]int64, error) {
	if s.listResUsersErr != nil {
		return nil, s.listResUsersErr
	}
	return s.AdminStore.ListReservationUsersByGame(gameId, limit)
}

// failingGameStore 内嵌真实 GameStore，仅 Get 可注入失败（visibleGame 路径）。
type failingGameStore struct {
	model.GameStore
	getErr error
}

var _ model.GameStore = (*failingGameStore)(nil)

func (s *failingGameStore) Get(id string) (*model.Game, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.GameStore.Get(id)
}

// 存储层失败 → 500：各 handler 的 ErrorCtx 臂。
func TestRoutes_StoreErrors(t *testing.T) {
	base, sign, svcCtx := newAuthedTestServerSvc(t, func(*config.Config) {})
	admin := sign(claimsFor(1, "admin"))
	player := sign(claimsFor(7, "bob"))
	origAdmin := svcCtx.AdminRepository
	origGame := svcCtx.GameRepository
	boom := errors.New("store down")

	cases := []struct {
		name     string
		method   string
		url      string
		token    string
		mutate   func()
		wantCode int
	}{
		{"list partners", http.MethodGet, base + "/admin/partners", admin,
			func() { svcCtx.AdminRepository = &failingAdminStore{AdminStore: origAdmin, listPartnersErr: boom} }, 500},
		{"list reviews", http.MethodGet, base + "/admin/reviews", admin,
			func() { svcCtx.AdminRepository = &failingAdminStore{AdminStore: origAdmin, listReviewsErr: boom} }, 500},
		{"list versions", http.MethodGet, base + "/admin/games/g-rpg-1/versions", admin,
			func() { svcCtx.AdminRepository = &failingAdminStore{AdminStore: origAdmin, listVersionsErr: boom} }, 500},
		{"me", http.MethodGet, base + "/me", admin,
			func() { svcCtx.AdminRepository = &failingAdminStore{AdminStore: origAdmin, getRoleErr: boom} }, 500},
		{"list audit", http.MethodGet, base + "/admin/audit", admin,
			func() { svcCtx.AdminRepository = &failingAdminStore{AdminStore: origAdmin, listAuditErr: boom} }, 500},
		{"list review reports", http.MethodGet, base + "/admin/review-reports", admin,
			func() { svcCtx.AdminRepository = &failingAdminStore{AdminStore: origAdmin, listReportsErr: boom} }, 500},
		{"list promo channels", http.MethodGet, base + "/admin/promo/channels", admin,
			func() { svcCtx.AdminRepository = &failingAdminStore{AdminStore: origAdmin, listPromoErr: boom} }, 500},
		{"channel stats", http.MethodGet, base + "/admin/promo/channels/c1/stats", admin,
			func() { svcCtx.AdminRepository = &failingAdminStore{AdminStore: origAdmin, channelStatsErr: boom} }, 500},
		{"my favorites", http.MethodGet, base + "/me/favorites", player,
			func() { svcCtx.AdminRepository = &failingAdminStore{AdminStore: origAdmin, listFavoritesErr: boom} }, 500},
		{"unfavorite", http.MethodDelete, base + "/games/g-rpg-1/favorite", player,
			func() { svcCtx.AdminRepository = &failingAdminStore{AdminStore: origAdmin, removeFavoriteErr: boom} }, 500},
		{"list game reviews", http.MethodGet, base + "/games/g-rpg-1/reviews", "",
			func() { svcCtx.GameRepository = &failingGameStore{GameStore: origGame, getErr: boom} }, 500},
		{"unvote review", http.MethodDelete, base + "/games/g-rpg-1/reviews/1/vote", player,
			func() { svcCtx.GameRepository = &failingGameStore{GameStore: origGame, getErr: boom} }, 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.mutate()
			code, _ := doAuthed(t, c.method, c.url, c.token, "")
			if code != c.wantCode {
				t.Fatalf("%s %s = %d, want %d", c.method, c.url, code, c.wantCode)
			}
		})
	}
}

// InternalResolveClick 存储层失败（GetPromoClick）→ 500，S2S 令牌通过。
func TestRoutes_InternalResolveStoreError(t *testing.T) {
	base, _, svcCtx := newAuthedTestServerSvc(t, func(c *config.Config) {
		c.Internal.Token = "s2s-token"
	})
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore: svcCtx.AdminRepository, getPromoClickErr: errors.New("store down"),
	}
	code, _ := doInternalResolve(t, base, `{"click_id":"c1","registered_at":"2026-01-01T00:00:00Z"}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("internal resolve store error = %d, want 500", code)
	}
}

// doInternalResolve 走 X-Internal-Token 守卫的 S2S 归因解析端点。
func doInternalResolve(t *testing.T, base, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, base+"/internal/promo/clicks/x/resolve",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", "s2s-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("internal resolve: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, readAll(t, resp)
}
