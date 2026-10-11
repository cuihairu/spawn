// Package hub 用户连接登记表与投递器：userId → 多连接（多端同时在线），
// 单机内存实现；跨实例投递经可选 Relay（Redis pub/sub）尽力而为广播。
package hub

import (
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Frame 推送帧格式：{"type":"notification","data":{...}}。
type Frame struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// watch/query 单次携带 id 上限与单客户端 watch 集合上限（内存有界）。
const (
	maxIDList         = 200
	maxWatchPerClient = 200
	presenceFrameType = "presence"
)

// PresenceStatus 单用户在线状态快照（query/REST 共用）。
type PresenceStatus struct {
	UserId      int64  `json:"user_id"`
	Online      bool   `json:"online"`
	Connections int    `json:"connections"`
	Since       string `json:"since,omitempty"`
}

// presenceEvt 上下线转换沿事件（推给 watch 者）。
type presenceEvt struct {
	UserId int64  `json:"user_id"`
	Online bool   `json:"online"`
	At     string `json:"at"`
}

// Client 一条 WS 连接。send 有界：满即丢弃并计数（背压不阻塞发布方）。
type Client struct {
	UserId int64

	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	watching map[int64]struct{} // 本连接 watch 的用户集合（h.mu 保护）
}

// Hub 连接登记表。
type Hub struct {
	queueSize  int
	writeWait  time.Duration
	pingPeriod time.Duration

	mu       sync.RWMutex
	conns    map[int64]map[*Client]struct{}
	since    map[int64]time.Time            // 首连时刻（在线期间保留）
	watchers map[int64]map[*Client]struct{} // 被 watch 用户 → watch 的连接
	drops    atomic.Int64                   // 溢出丢弃计数（并发投递无锁累计）
	relay    Relay                          // 跨实例投递（nil = 单实例）
	notifier Notifier                       // 离线推送出口（nil = 不推）
}

// SetRelay 挂跨实例投递器（启动期一次性调用，Serve 前完成）。
func (h *Hub) SetRelay(r Relay) { h.relay = r }

// SetNotifier 挂离线推送出口（启动期一次性调用，Serve 前完成）。
func (h *Hub) SetNotifier(n Notifier) { h.notifier = n }

// Options 可调参数（零值走默认）。
type Options struct {
	QueueSize  int
	WriteWait  time.Duration
	PingPeriod time.Duration
}

func (o Options) withDefaults() Options {
	if o.QueueSize <= 0 {
		o.QueueSize = 16
	}
	if o.WriteWait <= 0 {
		o.WriteWait = 10 * time.Second
	}
	if o.PingPeriod <= 0 {
		o.PingPeriod = 30 * time.Second
	}
	return o
}

// New 空登记表。
func New(opts Options) *Hub {
	o := opts.withDefaults()
	return &Hub{
		queueSize:  o.QueueSize,
		writeWait:  o.WriteWait,
		pingPeriod: o.PingPeriod,
		conns:      make(map[int64]map[*Client]struct{}),
		since:      make(map[int64]time.Time),
		watchers:   make(map[int64]map[*Client]struct{}),
	}
}

// Register 仅入表（不起读写泵；测试用）。首连（此前零连接）时记录
// since 并向 watch 者发 online 事件——多端第二连不触发。
func (h *Hub) Register(c *Client) bool {
	c.hub = h
	c.send = make(chan []byte, h.queueSize)
	c.watching = make(map[int64]struct{})
	h.mu.Lock()
	first := false
	set, ok := h.conns[c.UserId]
	if !ok {
		set = make(map[*Client]struct{})
		h.conns[c.UserId] = set
	}
	set[c] = struct{}{}
	if len(set) == 1 {
		h.since[c.UserId] = time.Now()
		first = true
	}
	h.mu.Unlock()
	if first {
		h.notifyPresence(c.UserId, true)
	}
	return first
}

// Join 入表并启动读写泵（生产路径）；online 事件由 Register 转换沿触发。
func (h *Hub) Join(userId int64, conn *websocket.Conn) *Client {
	c := &Client{UserId: userId, conn: conn}
	h.Register(c)
	go h.readPump(c)
	go h.writePump(c)
	return c
}

// Leave 出表并清 watch 关系；重复调用幂等。末断触发 offline 事件。
func (h *Hub) Leave(c *Client) {
	h.mu.Lock()
	offline := false
	if set, ok := h.conns[c.UserId]; ok {
		if _, in := set[c]; in {
			delete(set, c)
			if len(set) == 0 {
				delete(h.conns, c.UserId)
				delete(h.since, c.UserId)
				offline = true
			}
		}
	}
	// 摘除本连接的全部 watch 关系
	for id := range c.watching {
		if ws := h.watchers[id]; ws != nil {
			delete(ws, c)
			if len(ws) == 0 {
				delete(h.watchers, id)
			}
		}
	}
	c.watching = nil
	h.mu.Unlock()

	if offline {
		h.notifyPresence(c.UserId, false)
	}
}

// Watch 登记对若干用户的上下线关注；超上限静默截断，重复 watch 幂等。
func (h *Hub) Watch(c *Client, userIds []int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, id := range userIds {
		if _, dup := c.watching[id]; dup {
			continue
		}
		if len(c.watching) >= maxWatchPerClient {
			return
		}
		c.watching[id] = struct{}{}
		set, ok := h.watchers[id]
		if !ok {
			set = make(map[*Client]struct{})
			h.watchers[id] = set
		}
		set[c] = struct{}{}
	}
}

// Unwatch 解除关注；未 watch 过的 id 静默跳过。
func (h *Hub) Unwatch(c *Client, userIds []int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, id := range userIds {
		if _, ok := c.watching[id]; !ok {
			continue
		}
		delete(c.watching, id)
		if ws := h.watchers[id]; ws != nil {
			delete(ws, c)
			if len(ws) == 0 {
				delete(h.watchers, id)
			}
		}
	}
}

// Presence 在线快照（最多 maxIDList 个，重复 id 去重，保持入参顺序）。
func (h *Hub) Presence(userIds []int64) []PresenceStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(userIds) > maxIDList {
		userIds = userIds[:maxIDList]
	}
	out := make([]PresenceStatus, 0, len(userIds))
	seen := make(map[int64]struct{}, len(userIds))
	for _, id := range userIds {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		st := PresenceStatus{UserId: id, Connections: len(h.conns[id]), Online: len(h.conns[id]) > 0}
		if t, ok := h.since[id]; ok {
			st.Since = t.UTC().Format(time.RFC3339)
		}
		out = append(out, st)
	}
	return out
}

// Connections 某用户当前在线连接数（观测用）。
func (h *Hub) Connections(userId int64) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns[userId])
}

// Drops 累计溢出丢弃帧数（观测用）。
func (h *Hub) Drops() int64 {
	return h.drops.Load()
}

// SendToUsers 向给定用户的全部在线连接投递一帧，返回实际入队数；
// 队列满即丢弃计数，永不阻塞。挂了 Relay 时本地投递后广播给其他实例
// （本实例回环由 Relay 剔除，不重复投递）；单实例模式下零投递的用户
// 交给离线推送出口（多实例下本地零投递 ≠ 离线，不推）。
func (h *Hub) SendToUsers(userIds []int64, data []byte) int {
	delivered, missed := h.deliverLocal(userIds, data)
	if h.relay != nil {
		h.relay.Broadcast(userIds, data)
	} else if h.notifier != nil && len(missed) > 0 {
		h.notifier.NotifyOffline(missed, data)
	}
	return delivered
}

// DeliverLocal 本实例投递（Relay 回环回调），锁外调用安全。
func (h *Hub) DeliverLocal(userIds []int64, data []byte) { h.deliverLocal(userIds, data) }

// deliverLocal 本实例连接投递（持锁最小化，网络调用一律在锁外）；
// 返回投递数与本实例零投递的用户（入参去重）。
func (h *Hub) deliverLocal(userIds []int64, data []byte) (int, []int64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	delivered := 0
	counts := make(map[int64]int, len(userIds))
	order := make([]int64, 0, len(userIds))
	for _, id := range userIds {
		if _, dup := counts[id]; dup {
			continue // 重复 id 只投一次
		}
		order = append(order, id)
		counts[id] = 0
		for c := range h.conns[id] {
			select {
			case c.send <- data:
				delivered++
				counts[id]++
			default:
				h.drops.Add(1)
				log.Printf("send queue full, drop frame for user %d (total drops %d)", id, h.drops.Load())
			}
		}
	}
	missed := make([]int64, 0, len(order))
	for _, id := range order {
		if counts[id] == 0 {
			missed = append(missed, id)
		}
	}
	return delivered, missed
}

// notifyPresence 上下线转换沿事件：快照 watch 者后锁外投递（非阻塞）。
func (h *Hub) notifyPresence(userId int64, online bool) {
	h.mu.RLock()
	watchers := make([]*Client, 0, len(h.watchers[userId]))
	for c := range h.watchers[userId] {
		watchers = append(watchers, c)
	}
	h.mu.RUnlock()
	if len(watchers) == 0 {
		return
	}
	evt, err := json.Marshal(presenceEvt{UserId: userId, Online: online, At: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		return
	}
	frame, err := json.Marshal(Frame{Type: presenceFrameType, Data: evt})
	if err != nil {
		return
	}
	h.sendToClients(watchers, frame)
}

// sendToClients 向指定连接非阻塞投递；队列满丢弃计数。
func (h *Hub) sendToClients(clients []*Client, data []byte) {
	for _, c := range clients {
		select {
		case c.send <- data:
		default:
			h.drops.Add(1)
			log.Printf("send queue full, drop frame (total drops %d)", h.drops.Load())
		}
	}
}

// clientMsg 客户端上行控制消息（watch/unwatch/query）。
type clientMsg struct {
	Action  string  `json:"action"`
	UserIds []int64 `json:"user_ids"`
}

// readPump 消费客户端上行：watch/unwatch/query 三类控制消息，
// 其余静默忽略（畸形 JSON 同样忽略，连接不断）；读超时由 pong 刷新。
func (h *Hub) readPump(c *Client) {
	defer h.Leave(c)
	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(2 * h.pingPeriod))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(2 * h.pingPeriod))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		h.handleClientMsg(c, data)
	}
}

// handleClientMsg 路由客户端控制消息；未知 action 与畸形 JSON 静默忽略。
func (h *Hub) handleClientMsg(c *Client, data []byte) {
	var msg clientMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	switch msg.Action {
	case "watch":
		h.Watch(c, msg.UserIds)
	case "unwatch":
		h.Unwatch(c, msg.UserIds)
	case "query":
		h.replyPresence(c, msg.UserIds)
	}
}

// replyPresence 只向请求连接回一帧在线快照（终端无内部令牌，
// 拉取走 WS 既有 JWT 鉴权通道）。
func (h *Hub) replyPresence(c *Client, userIds []int64) {
	items := h.Presence(userIds)
	data, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		return
	}
	frame, err := json.Marshal(Frame{Type: presenceFrameType, Data: data})
	if err != nil {
		return
	}
	h.sendToClients([]*Client{c}, frame)
}

// writePump 出队写帧 + 周期 ping；写失败即断开出表。
func (h *Hub) writePump(c *Client) {
	ticker := time.NewTicker(h.pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case data, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(h.writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(h.writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
