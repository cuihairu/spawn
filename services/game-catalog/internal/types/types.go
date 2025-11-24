package types

// Game 定义客户端可见的游戏结构
type Game struct {
	Id            string   `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Genres        []string `json:"genres"`
	Platforms     []string `json:"platforms"`
	ReleaseDate   string   `json:"release_date"`
	Developer     string   `json:"developer"`
	Publisher     string   `json:"publisher"`
	Tags          []string `json:"tags"`
	Score         float64  `json:"score"`
	TrendingScore int      `json:"trending_score"`
	CoverImage    string   `json:"cover_image"`
}

type ListGamesRequest struct {
	Keyword  string `form:"keyword,optional"`
	Genre    string `form:"genre,optional"`
	Platform string `form:"platform,optional"`
	Tag      string `form:"tag,optional"`
	Limit    int64  `form:"limit,default=20"`
	Offset   int64  `form:"offset,default=0"`
	Sort     string `form:"sort,default=popularity"`
}

type ListGamesResponse struct {
	Games  []Game `json:"games"`
	Total  int    `json:"total"`
	Limit  int64  `json:"limit"`
	Offset int64  `json:"offset"`
}

type GetGameDetailRequest struct {
	Id string `path:"id"`
}

type FeaturedGamesRequest struct {
	Limit int64 `form:"limit,default=6"`
}

type RecommendationsRequest struct {
	UserId string `form:"userId,optional"`
	Genres string `form:"genres,optional"`
	Limit  int64  `form:"limit,default=5"`
}

type RecommendationsResponse struct {
	Games []Game `json:"games"`
}

type CreateGameRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Genres      []string `json:"genres"`
	Platforms   []string `json:"platforms"`
	ReleaseDate string   `json:"release_date"`
	Developer   string   `json:"developer"`
	Publisher   string   `json:"publisher"`
	Tags        []string `json:"tags"`
	Score       float64  `json:"score"`
	CoverImage  string   `json:"cover_image"`
}

type GameDetailResponse struct {
	Game Game `json:"game"`
}

type FeaturedGamesResponse struct {
	Games []Game `json:"games"`
}
