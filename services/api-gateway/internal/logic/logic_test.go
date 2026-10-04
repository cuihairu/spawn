package logic

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/api-gateway/internal/integration"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/tappi/tappi/services/api-gateway/internal/types"
)

type mockUserService struct {
	loginFn func(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error)
	recoFn  func(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error)
}

func (m mockUserService) Login(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error) {
	if m.loginFn == nil {
		return nil, errors.New("loginFn not implemented")
	}
	return m.loginFn(ctx, payload)
}

func (m mockUserService) GetRecommendations(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error) {
	if m.recoFn == nil {
		return nil, errors.New("recoFn not implemented")
	}
	return m.recoFn(ctx, userID, limit, genres, token)
}

type mockGameCatalog struct {
	featuredFn func(ctx context.Context, limit int64) (map[string]interface{}, error)
}

func (m mockGameCatalog) GetFeatured(ctx context.Context, limit int64) (map[string]interface{}, error) {
	if m.featuredFn == nil {
		return nil, errors.New("featuredFn not implemented")
	}
	return m.featuredFn(ctx, limit)
}

func TestLoginLogicSuccess(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: mockUserService{
			loginFn: func(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error) {
				if payload.Username != "demo" || payload.Password != "secret" {
					t.Fatalf("unexpected payload: %#v", payload)
				}
				return &integration.LoginResult{
					Token:    "token-123",
					UserInfo: map[string]any{"username": payload.Username},
				}, nil
			},
			recoFn: func(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error) {
				return nil, errors.New("not expected")
			},
		},
	}

	logic := NewLoginLogic(context.Background(), svcCtx)
	resp, err := logic.Login(&types.LoginRequest{Username: "demo", Password: "secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Token != "token-123" {
		t.Fatalf("expected token value, got %s", resp.Token)
	}
}

func TestLoginLogicError(t *testing.T) {
	expected := errors.New("upstream error")
	svcCtx := &svc.ServiceContext{
		UserService: mockUserService{
			loginFn: func(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error) {
				return nil, expected
			},
			recoFn: func(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error) {
				return nil, nil
			},
		},
	}

	logic := NewLoginLogic(context.Background(), svcCtx)
	if _, err := logic.Login(&types.LoginRequest{}); !errors.Is(err, expected) {
		t.Fatalf("expected error %v, got %v", expected, err)
	}
}

func TestFeaturedGamesLogicUsesDefaultLimit(t *testing.T) {
	var capturedLimit int64
	svcCtx := &svc.ServiceContext{
		GameCatalog: mockGameCatalog{
			featuredFn: func(ctx context.Context, limit int64) (map[string]interface{}, error) {
				capturedLimit = limit
				return map[string]interface{}{
					"games": []interface{}{"a", "b"},
				}, nil
			},
		},
		UserService: mockUserService{
			loginFn: func(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error) {
				return nil, nil
			},
			recoFn: func(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error) {
				return nil, nil
			},
		},
	}

	logic := NewFeaturedGamesLogic(context.Background(), svcCtx)
	resp, err := logic.FeaturedGames(&types.FeaturedRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 6 {
		t.Fatalf("expected default limit 6, got %d", capturedLimit)
	}
	if len(resp.Games) != 2 {
		t.Fatalf("expected 2 games, got %d", len(resp.Games))
	}
}

func TestUserRecommendationsLogic(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: mockUserService{
			loginFn: func(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error) {
				return nil, nil
			},
			recoFn: func(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error) {
				if token != "token-abc" || userID != 42 || limit != 10 || genres != "RPG" {
					t.Fatalf("unexpected params: user=%d limit=%d genres=%s token=%s", userID, limit, genres, token)
				}
				return &integration.RecommendationResponse{
					Code:    200,
					Message: "ok",
					Data: map[string]any{
						"recommendations": []string{"game-1"},
					},
				}, nil
			},
		},
		GameCatalog: mockGameCatalog{
			featuredFn: func(ctx context.Context, limit int64) (map[string]interface{}, error) {
				return nil, nil
			},
		},
	}

	logic := NewUserRecommendationsLogic(context.Background(), svcCtx)
	resp, err := logic.UserRecommendations(&types.RecommendationRequest{
		Id:     42,
		Limit:  10,
		Genres: "RPG",
	}, "token-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != 200 || resp.Message != "ok" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok || len(data) == 0 {
		t.Fatalf("unexpected data type: %#v", resp.Data)
	}
}

type mockCommunity struct {
	getPostFn  func(ctx context.Context, id int64) (*integration.CommunityPost, error)
	hotPostsFn func(ctx context.Context, limit int) ([]integration.CommunityPostSummary, error)
	topicsFn   func(ctx context.Context, limit int) ([]integration.CommunityTopicSummary, error)
}

func (m mockCommunity) GetPost(ctx context.Context, id int64) (*integration.CommunityPost, error) {
	if m.getPostFn == nil {
		return nil, errors.New("getPostFn not implemented")
	}
	return m.getPostFn(ctx, id)
}

func (m mockCommunity) ListHotPosts(ctx context.Context, limit int) ([]integration.CommunityPostSummary, error) {
	if m.hotPostsFn == nil {
		return nil, errors.New("hotPostsFn not implemented")
	}
	return m.hotPostsFn(ctx, limit)
}

func (m mockCommunity) ListTopics(ctx context.Context, limit int) ([]integration.CommunityTopicSummary, error) {
	if m.topicsFn == nil {
		return nil, errors.New("topicsFn not implemented")
	}
	return m.topicsFn(ctx, limit)
}

type mockContent struct {
	listGuidesFn func(ctx context.Context, limit int) ([]integration.GuideSummary, error)
}

func (m mockContent) ListGuides(ctx context.Context, limit int) ([]integration.GuideSummary, error) {
	if m.listGuidesFn == nil {
		return nil, errors.New("listGuidesFn not implemented")
	}
	return m.listGuidesFn(ctx, limit)
}

func TestHomeFeedLogicAggregatesAndTrims(t *testing.T) {
	var gotGameLimit, gotPostLimit, gotTopicLimit, gotGuideLimit int
	svcCtx := &svc.ServiceContext{
		GameCatalog: mockGameCatalog{
			featuredFn: func(ctx context.Context, limit int64) (map[string]interface{}, error) {
				gotGameLimit = int(limit)
				return map[string]interface{}{
					"games": []interface{}{
						map[string]interface{}{
							"id": "g1", "title": "星陨物语", "description": "不该下发的长简介",
							"developer": "不该下发", "cover_image": "https://cdn/cover/g1.png",
							"score": 9.1, "genres": []interface{}{"RPG", "剧情"}, "platforms": []interface{}{"PC", "Switch"},
						},
						"not-a-map", // 非 map 条目跳过
					},
				}, nil
			},
		},
		Community: mockCommunity{
			hotPostsFn: func(ctx context.Context, limit int) ([]integration.CommunityPostSummary, error) {
				gotPostLimit = limit
				return []integration.CommunityPostSummary{{
					Id: 11, TopicId: 2, AuthorId: 7, AuthorName: "alice",
					Title: "通关心得", Content: strings.Repeat("好", 100) + "\n第二行",
					LikeCount: 12, CommentCount: 3, CreatedAt: "2026-10-04T10:00:00Z",
				}}, nil
			},
			topicsFn: func(ctx context.Context, limit int) ([]integration.CommunityTopicSummary, error) {
				gotTopicLimit = limit
				return []integration.CommunityTopicSummary{{
					Id: 2, Name: "星陨圈", PostCount: 30, FollowerCount: 88, IsOfficial: true,
				}}, nil
			},
		},
		Content: mockContent{
			listGuidesFn: func(ctx context.Context, limit int) ([]integration.GuideSummary, error) {
				gotGuideLimit = limit
				return []integration.GuideSummary{{
					Id: 5, GameId: "g1", GameTitle: "星陨物语", Title: "全收集攻略",
					Summary: "官方摘要", AuthorName: "bob", Likes: 9, Views: 120,
					CreatedAt: "2026-10-01T08:00:00Z",
				}}, nil
			},
		},
	}

	resp, err := NewHomeFeedLogic(context.Background(), svcCtx).HomeFeed(&types.HomeFeedRequest{Limit: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotGameLimit != 3 || gotPostLimit != 3 || gotTopicLimit != 3 || gotGuideLimit != 3 {
		t.Fatalf("limits = game %d post %d topic %d guide %d, want all 3",
			gotGameLimit, gotPostLimit, gotTopicLimit, gotGuideLimit)
	}
	if len(resp.FeaturedGames) != 1 || len(resp.HotPosts) != 1 || len(resp.Topics) != 1 || len(resp.Guides) != 1 {
		t.Fatalf("group lengths = %d/%d/%d/%d, want 1/1/1/1",
			len(resp.FeaturedGames), len(resp.HotPosts), len(resp.Topics), len(resp.Guides))
	}
	if len(resp.Degraded) != 0 {
		t.Fatalf("degraded = %v, want empty", resp.Degraded)
	}

	game := resp.FeaturedGames[0]
	if game.Id != "g1" || game.Title != "星陨物语" || game.CoverImage != "https://cdn/cover/g1.png" || game.Score != 9.1 {
		t.Fatalf("game = %+v", game)
	}
	if len(game.Genres) != 2 || len(game.Platforms) != 2 {
		t.Fatalf("game slices = %v %v", game.Genres, game.Platforms)
	}

	// 字段裁剪：description/developer 等大字段不在响应结构里（类型层面保证），
	// 这里断言裁剪后的摘要：正文压成单行并截断到 60 rune + …
	post := resp.HotPosts[0]
	wantSummary := strings.Repeat("好", 60) + "…"
	if post.Summary != wantSummary {
		t.Fatalf("post summary len = %d, want %d runes ending with ellipsis", len([]rune(post.Summary)), 61)
	}
	if post.Id != 11 || post.AuthorName != "alice" || post.LikeCount != 12 || post.CommentCount != 3 {
		t.Fatalf("post = %+v", post)
	}
	topic := resp.Topics[0]
	if topic.Id != 2 || topic.Name != "星陨圈" || !topic.IsOfficial || topic.PostCount != 30 || topic.FollowerCount != 88 {
		t.Fatalf("topic = %+v", topic)
	}
	guide := resp.Guides[0]
	if guide.Id != 5 || guide.Title != "全收集攻略" || guide.Summary != "官方摘要" || guide.Views != 120 {
		t.Fatalf("guide = %+v", guide)
	}
}

func TestHomeFeedLogicDegradesPerGroup(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		GameCatalog: mockGameCatalog{
			featuredFn: func(ctx context.Context, limit int64) (map[string]interface{}, error) {
				return nil, errors.New("game-catalog down")
			},
		},
		Community: mockCommunity{
			hotPostsFn: func(ctx context.Context, limit int) ([]integration.CommunityPostSummary, error) {
				return nil, errors.New("community down")
			},
			topicsFn: func(ctx context.Context, limit int) ([]integration.CommunityTopicSummary, error) {
				return []integration.CommunityTopicSummary{{Id: 1, Name: "still-up"}}, nil
			},
		},
		Content: mockContent{
			listGuidesFn: func(ctx context.Context, limit int) ([]integration.GuideSummary, error) {
				return nil, errors.New("content down")
			},
		},
	}

	resp, err := NewHomeFeedLogic(context.Background(), svcCtx).HomeFeed(&types.HomeFeedRequest{})
	if err != nil {
		t.Fatalf("aggregate endpoint must not 5xx on single upstream failure: %v", err)
	}
	if len(resp.FeaturedGames) != 0 || len(resp.HotPosts) != 0 || len(resp.Guides) != 0 {
		t.Fatalf("failed groups must be empty lists")
	}
	if len(resp.Topics) != 1 || resp.Topics[0].Name != "still-up" {
		t.Fatalf("healthy group must still serve: %+v", resp.Topics)
	}
	sort.Strings(resp.Degraded)
	want := []string{"featured_games", "guides", "hot_posts"}
	if !reflect.DeepEqual(resp.Degraded, want) {
		t.Fatalf("degraded = %v, want %v", resp.Degraded, want)
	}
}

func TestHomeFeedLogicLimitClamp(t *testing.T) {
	var gotLimit int
	svcCtx := &svc.ServiceContext{
		GameCatalog: mockGameCatalog{
			featuredFn: func(ctx context.Context, limit int64) (map[string]interface{}, error) {
				gotLimit = int(limit)
				return map[string]interface{}{"games": []interface{}{}}, nil
			},
		},
		Community: mockCommunity{
			hotPostsFn: func(ctx context.Context, limit int) ([]integration.CommunityPostSummary, error) {
				return nil, nil
			},
			topicsFn: func(ctx context.Context, limit int) ([]integration.CommunityTopicSummary, error) {
				return nil, nil
			},
		},
		Content: mockContent{
			listGuidesFn: func(ctx context.Context, limit int) ([]integration.GuideSummary, error) {
				return nil, nil
			},
		},
	}

	// 缺省 → 5
	if _, err := NewHomeFeedLogic(context.Background(), svcCtx).HomeFeed(&types.HomeFeedRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != homeFeedDefaultLimit {
		t.Fatalf("default limit = %d, want %d", gotLimit, homeFeedDefaultLimit)
	}
	// 超上限 → 20
	if _, err := NewHomeFeedLogic(context.Background(), svcCtx).HomeFeed(&types.HomeFeedRequest{Limit: 999}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != homeFeedMaxLimit {
		t.Fatalf("clamped limit = %d, want %d", gotLimit, homeFeedMaxLimit)
	}
}

// TestHomeFeedLogicDegradesTopics 与 TestHomeFeedLogicDegradesPerGroup 互补：
// 那条用例固定 topics 健康（证明健康组不受牵连），这条让 topics 失败，
// 触达 topics 组的降级分支。
func TestHomeFeedLogicDegradesTopics(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		GameCatalog: mockGameCatalog{
			featuredFn: func(ctx context.Context, limit int64) (map[string]interface{}, error) {
				return map[string]interface{}{"games": []interface{}{}}, nil
			},
		},
		Community: mockCommunity{
			hotPostsFn: func(ctx context.Context, limit int) ([]integration.CommunityPostSummary, error) {
				return nil, nil
			},
			topicsFn: func(ctx context.Context, limit int) ([]integration.CommunityTopicSummary, error) {
				return nil, errors.New("community topics down")
			},
		},
		Content: mockContent{
			listGuidesFn: func(ctx context.Context, limit int) ([]integration.GuideSummary, error) {
				return nil, nil
			},
		},
	}

	resp, err := NewHomeFeedLogic(context.Background(), svcCtx).HomeFeed(&types.HomeFeedRequest{})
	if err != nil {
		t.Fatalf("aggregate endpoint must not 5xx on single upstream failure: %v", err)
	}
	if len(resp.Topics) != 0 {
		t.Fatalf("failed group must be empty list")
	}
	if !reflect.DeepEqual(resp.Degraded, []string{"topics"}) {
		t.Fatalf("degraded = %v, want [topics]", resp.Degraded)
	}
}

func TestSummarizeForFeed(t *testing.T) {
	if got := summarizeForFeed(""); got != "" {
		t.Fatalf("empty content summary = %q, want empty", got)
	}
	// 短正文原样返回且压成单行（换行/连续空白折叠）。
	short := summarizeForFeed("第一行\n\n第二行\t细节")
	if short != "第一行 第二行 细节" {
		t.Fatalf("short summary = %q", short)
	}
	// 超长截断到 60 rune + 省略号。
	long := summarizeForFeed(strings.Repeat("长", 80))
	if got := []rune(long); len(got) != 61 || string(got[60]) != "…" {
		t.Fatalf("long summary = %d runes, want 61 ending with ellipsis", len(got))
	}
}
