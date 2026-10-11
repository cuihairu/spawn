package hub

import "encoding/json"

// signalFrameType 信令下行帧类型：data 内嵌 {from, room, data}，
// data 为 WebRTC offer/answer/ICE candidate 等（hub 不解析不存状态）。
const signalFrameType = "signal"

// signalPayload 信令转发体：room 是端上协商的关联 id（原样透传）。
type signalPayload struct {
	From int64           `json:"from"`
	Room string          `json:"room,omitempty"`
	Data json.RawMessage `json:"data"`
}

// relaySignal 把一条信令转发给目标用户的全部在线连接（发端连接除外，
// 本端多设备自信号不回环）；Relay 模式下跨实例兜底广播（发布方回环
// 由 Relay 剔除）。无状态、尽力而为：目标不在线即静默丢弃，重试由
// 端上协商层负责；尺寸上限即上行读限 4096。
func (h *Hub) relaySignal(c *Client, msg clientMsg) {
	if msg.To <= 0 {
		return
	}
	payload, err := json.Marshal(signalPayload{From: c.UserId, Room: msg.Room, Data: msg.Data})
	if err != nil {
		return
	}
	frame, err := json.Marshal(Frame{Type: signalFrameType, Data: payload})
	if err != nil {
		return
	}
	h.sendToConnsOf(msg.To, c, frame)
	if h.relay != nil {
		h.relay.Broadcast([]int64{msg.To}, frame)
	}
}

// sendToConnsOf 某用户的全部连接（可排除一条，如发端自身）非阻塞投递。
func (h *Hub) sendToConnsOf(userId int64, exclude *Client, data []byte) {
	h.mu.RLock()
	targets := make([]*Client, 0, len(h.conns[userId]))
	for c := range h.conns[userId] {
		if c != exclude {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()
	h.sendToClients(targets, data)
}
