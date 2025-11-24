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
