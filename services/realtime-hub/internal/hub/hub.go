// Package hub 用户连接登记表与投递器：userId → 多连接（多端同时在线），
// 单机内存实现；水平扩展（Redis pub/sub）为 M3 远期项。
package hub

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Frame 推送帧格式：{"type":"notification","data":{...}}。
type Frame struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Client 一条 WS 连接。send 有界：满即丢弃并计数（背压不阻塞发布方）。
type Client struct {
	UserId int64

	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Hub 连接登记表。
type Hub struct {
	queueSize  int
	writeWait  time.Duration
	pingPeriod time.Duration

	mu    sync.RWMutex
	conns map[int64]map[*Client]struct{}
	drops int64 // 溢出丢弃计数（mu 保护）
}

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
	}
}

// Register 仅入表（不起读写泵；测试用）。
func (h *Hub) Register(c *Client) {
	c.hub = h
	c.send = make(chan []byte, h.queueSize)
	h.mu.Lock()
	defer h.mu.Unlock()
	set, ok := h.conns[c.UserId]
	if !ok {
		set = make(map[*Client]struct{})
		h.conns[c.UserId] = set
	}
	set[c] = struct{}{}
}

// Join 入表并启动读写泵（生产路径）。
func (h *Hub) Join(userId int64, conn *websocket.Conn) *Client {
	c := &Client{UserId: userId, conn: conn}
	h.Register(c)
	go h.readPump(c)
	go h.writePump(c)
	return c
}

// Leave 出表；重复调用幂等。
func (h *Hub) Leave(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set, ok := h.conns[c.UserId]
	if !ok {
		return
	}
	if _, ok := set[c]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.conns, c.UserId)
		}
	}
}

// Connections 某用户当前在线连接数（观测用）。
func (h *Hub) Connections(userId int64) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns[userId])
}

// Drops 累计溢出丢弃帧数（观测用）。
func (h *Hub) Drops() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.drops
}

// SendToUsers 向给定用户的全部在线连接投递一帧，返回实际入队数；
// 队列满即丢弃计数，永不阻塞。
func (h *Hub) SendToUsers(userIds []int64, data []byte) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	delivered := 0
	for _, id := range userIds {
		for c := range h.conns[id] {
			select {
			case c.send <- data:
				delivered++
			default:
				h.drops++
				log.Printf("send queue full, drop frame for user %d (total drops %d)", id, h.drops)
			}
		}
	}
	return delivered
}

// readPump 消费客户端上行（M1 无客户端语义，读出即弃）；读超时由 pong 刷新。
func (h *Hub) readPump(c *Client) {
	defer h.Leave(c)
	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(2 * h.pingPeriod))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(2 * h.pingPeriod))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
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
