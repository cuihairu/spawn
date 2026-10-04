package logic

import (
	"context"
	"strings"
	"sync"

	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/tappi/tappi/services/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// homeFeedLimit 边界：缺省每组 5 条，上限 20（一屏聚合的合理上界）。
const (
	homeFeedDefaultLimit = 5
	homeFeedMaxLimit     = 20
	// homeFeedSummaryRunes 热帖摘要截断长度（列表展示口径，全文仍以帖子详情为准）。
	homeFeedSummaryRunes = 60
)

type HomeFeedLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewHomeFeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HomeFeedLogic {
	return &HomeFeedLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// HomeFeed BFF 第二阶段聚合端点：一次请求并发拉取精选游戏 / 热帖 / 话题 / 攻略
// 四个列表，逐组做字段裁剪（正文、图片等大字段不下发，正文仅截断出摘要）。
// 单个上游失败只降级该组为空列表并记入 degraded，不做整体 5xx。
func (l *HomeFeedLogic) HomeFeed(req *types.HomeFeedRequest) (*types.HomeFeedResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = homeFeedDefaultLimit
	}
	if limit > homeFeedMaxLimit {
		limit = homeFeedMaxLimit
	}

	resp := &types.HomeFeedResponse{
		FeaturedGames: []types.HomeFeedGame{},
		HotPosts:      []types.HomeFeedPost{},
		Topics:        []types.HomeFeedTopic{},
		Guides:        []types.HomeFeedGuide{},
		Degraded:      []string{},
	}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	degrade := func(group string, err error) {
		l.Errorf("homefeed group %s degraded: %v", group, err)
		mu.Lock()
		resp.Degraded = append(resp.Degraded, group)
		mu.Unlock()
	}

	wg.Add(4)
	go func() {
		defer wg.Done()
		payload, err := l.svcCtx.GameCatalog.GetFeatured(l.ctx, limit)
		if err != nil {
			degrade("featured_games", err)
			return
		}
		rawGames, _ := payload["games"].([]interface{})
		for _, raw := range rawGames {
			if g, ok := homeFeedGameFromRaw(raw); ok {
				resp.FeaturedGames = append(resp.FeaturedGames, g)
			}
		}
	}()

	go func() {
		defer wg.Done()
		posts, err := l.svcCtx.Community.ListHotPosts(l.ctx, int(limit))
		if err != nil {
			degrade("hot_posts", err)
			return
		}
		for _, p := range posts {
			resp.HotPosts = append(resp.HotPosts, types.HomeFeedPost{
				Id:           p.Id,
				TopicId:      p.TopicId,
				AuthorId:     p.AuthorId,
				AuthorName:   p.AuthorName,
				Title:        p.Title,
				Summary:      summarizeForFeed(p.Content),
				LikeCount:    p.LikeCount,
				CommentCount: p.CommentCount,
				CreatedAt:    p.CreatedAt,
			})
		}
	}()

	go func() {
		defer wg.Done()
		topics, err := l.svcCtx.Community.ListTopics(l.ctx, int(limit))
		if err != nil {
			degrade("topics", err)
			return
		}
		for _, t := range topics {
			resp.Topics = append(resp.Topics, types.HomeFeedTopic{
				Id:            t.Id,
				Name:          t.Name,
				PostCount:     t.PostCount,
				FollowerCount: t.FollowerCount,
				IsOfficial:    t.IsOfficial,
			})
		}
	}()

	go func() {
		defer wg.Done()
		guides, err := l.svcCtx.Content.ListGuides(l.ctx, int(limit))
		if err != nil {
			degrade("guides", err)
			return
		}
		for _, g := range guides {
			resp.Guides = append(resp.Guides, types.HomeFeedGuide{
				Id:         g.Id,
				GameId:     g.GameId,
				GameTitle:  g.GameTitle,
				Title:      g.Title,
				Summary:    g.Summary,
				AuthorName: g.AuthorName,
				Likes:      g.Likes,
				Views:      g.Views,
				CreatedAt:  g.CreatedAt,
			})
		}
	}()

	wg.Wait()
	return resp, nil
}

// homeFeedGameFromRaw 从精选游戏原始 JSON（map）里挑裁剪字段；
// 非 map 条目跳过而不是让整组失败。
func homeFeedGameFromRaw(raw interface{}) (types.HomeFeedGame, bool) {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return types.HomeFeedGame{}, false
	}
	g := types.HomeFeedGame{}
	g.Id, _ = m["id"].(string)
	g.Title, _ = m["title"].(string)
	g.CoverImage, _ = m["cover_image"].(string)
	g.Score, _ = m["score"].(float64)
	g.Genres = stringSliceFromRaw(m["genres"])
	g.Platforms = stringSliceFromRaw(m["platforms"])
	return g, true
}

func stringSliceFromRaw(raw interface{}) []string {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// summarizeForFeed 把帖子正文压成单行截断摘要；空正文返回空串。
func summarizeForFeed(content string) string {
	oneLine := strings.Join(strings.Fields(content), " ")
	runes := []rune(oneLine)
	if len(runes) <= homeFeedSummaryRunes {
		return oneLine
	}
	return string(runes[:homeFeedSummaryRunes]) + "…"
}
