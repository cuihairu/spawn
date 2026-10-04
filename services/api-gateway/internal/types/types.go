package types

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token    string      `json:"token"`
	UserInfo interface{} `json:"user_info"`
}

type FeaturedRequest struct {
	Limit int64 `form:"limit,default=6"`
}

type FeaturedResponse struct {
	Games []interface{} `json:"games"`
}

type RecommendationRequest struct {
	Id     int64  `path:"id"`
	Limit  int64  `form:"limit,optional"`
	Genres string `form:"genres,optional"`
}

type RecommendationResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type HomeFeedRequest struct {
	Limit int64 `form:"limit,optional"`
}

// HomeFeed 系列是 BFF 聚合端点（GET /home/feed）的裁剪响应口径：
// 每组仅保留列表展示所需字段，正文/图片等大字段不下发。

type HomeFeedGame struct {
	Id         string   `json:"id"`
	Title      string   `json:"title"`
	CoverImage string   `json:"cover_image"`
	Score      float64  `json:"score"`
	Genres     []string `json:"genres"`
	Platforms  []string `json:"platforms"`
}

type HomeFeedPost struct {
	Id           int64  `json:"id"`
	TopicId      int64  `json:"topic_id"`
	AuthorId     int64  `json:"author_id"`
	AuthorName   string `json:"author_name"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	LikeCount    int64  `json:"like_count"`
	CommentCount int64  `json:"comment_count"`
	CreatedAt    string `json:"created_at"`
}

type HomeFeedTopic struct {
	Id            int64  `json:"id"`
	Name          string `json:"name"`
	PostCount     int64  `json:"post_count"`
	FollowerCount int64  `json:"follower_count"`
	IsOfficial    bool   `json:"is_official"`
}

type HomeFeedGuide struct {
	Id         int64  `json:"id"`
	GameId     string `json:"game_id"`
	GameTitle  string `json:"game_title"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	AuthorName string `json:"author_name"`
	Likes      int64  `json:"likes"`
	Views      int64  `json:"views"`
	CreatedAt  string `json:"created_at"`
}

type HomeFeedResponse struct {
	FeaturedGames []HomeFeedGame  `json:"featured_games"`
	HotPosts      []HomeFeedPost  `json:"hot_posts"`
	Topics        []HomeFeedTopic `json:"topics"`
	Guides        []HomeFeedGuide `json:"guides"`
	// Degraded 列出因上游失败被降级为空列表的分组（缺省为空数组），
	// 聚合端点不因单一上游故障整体 5xx。
	Degraded []string `json:"degraded"`
}
