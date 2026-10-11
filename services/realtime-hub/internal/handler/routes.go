// Package handler realtime-hub 四个端点：/health 探活、/ws 客户端握手
// （JWT 校验后入 registry，支持 watch/unwatch/query 控制消息）、
// /internal/publish 服务间投递、/internal/presence 服务间在线查询
// （均 X-Internal-Token 鉴权，未配置恒 403）。
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/tappi/tappi/services/realtime-hub/internal/auth"
	"github.com/tappi/tappi/services/realtime-hub/internal/hub"
)

// publishReq 服务间投递请求体。
type publishReq struct {
	UserIds []int64      `json:"user_ids"`
	Event   publishEvent `json:"event"`
}

type publishEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// publishResp 投递结果（delivered = 实际入队连接数）。
type publishResp struct {
	Delivered int `json:"delivered"`
}

// presenceResp 在线查询结果（与入参同序，重复 id 去重）。
type presenceResp struct {
	Presence []hub.PresenceStatus `json:"presence"`
}

// Server 路由装配。
type Server struct {
	hub           *hub.Hub
	verifier      *auth.Verifier
	internalToken string
	upgrader      websocket.Upgrader
}

func NewServer(h *hub.Hub, v *auth.Verifier, internalToken string) http.Handler {
	s := &Server{
		hub:           h,
		verifier:      v,
		internalToken: internalToken,
		upgrader: websocket.Upgrader{
			ReadBufferSize: 1024, WriteBufferSize: 1024,
			// JWT query 是唯一准入闸：令牌不随跨域自动携带，Origin 校验
			// 对非浏览器客户端无意义、只会误伤浏览器（web :4173 跨源连
			// hub/网关隧道），本地部署口径放开。
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /ws", s.ws)
	mux.HandleFunc("POST /internal/publish", s.publish)
	mux.HandleFunc("GET /internal/presence", s.presence)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ws 浏览器 WS 无法带自定义 header，握手 token 走 query（本地部署口径）。
// 校验失败在升级前拒绝（纯 HTTP 401），不产生半开连接。
func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	claims, err := s.verifier.ParseToken(r.URL.Query().Get("token"))
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade 已写回错误响应
	}
	s.hub.Join(claims.UserId, conn)
}

// publish 服务间投递：内部令牌未配置时恒 403（默认拒绝）；投递为
// fire-and-forget 语义——入队失败只计数，不回错。
func (s *Server) publish(w http.ResponseWriter, r *http.Request) {
	if s.internalToken == "" || r.Header.Get("X-Internal-Token") != s.internalToken {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req publishReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if len(req.UserIds) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_ids required"})
		return
	}
	if req.Event.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event type required"})
		return
	}

	frame, err := json.Marshal(hub.Frame{Type: req.Event.Type, Data: req.Event.Payload})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "marshal frame"})
		return
	}

	writeJSON(w, http.StatusOK, publishResp{Delivered: s.hub.SendToUsers(req.UserIds, frame)})
}

// presence 服务间在线查询：user_ids 逗号分隔（最多 200 个，超出截断）；
// 内部令牌未配置恒 403（fail closed 同 publish）。
func (s *Server) presence(w http.ResponseWriter, r *http.Request) {
	if s.internalToken == "" || r.Header.Get("X-Internal-Token") != s.internalToken {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	raw := r.URL.Query().Get("user_ids")
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_ids required"})
		return
	}
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user_ids"})
			return
		}
		ids = append(ids, id)
	}

	writeJSON(w, http.StatusOK, presenceResp{Presence: s.hub.Presence(ids)})
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
