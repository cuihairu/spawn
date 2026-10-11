package hub

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// Relay 跨实例投递出口：本地投递后经此广播到其他实例（nil = 单实例内存
// 模式，现状语义）。实现方须剔除发布方自己的回环（本地已投递过）。
type Relay interface {
	Broadcast(userIds []int64, frame []byte)
	Close() error
}

// RedisChannel 订阅频道名（所有实例共用）。
const RedisChannel = "realtime-hub:events"

// redisEnvelope 通道载荷：frame 内嵌原始推送帧。
type redisEnvelope struct {
	From    string  `json:"from"`
	UserIds []int64 `json:"user_ids"`
	Frame   []byte  `json:"frame"`
}

// RedisRelay Redis pub/sub 实现。发布超时/订阅断连只记日志——跨实例
// 投递是尽力而为语义，本地投递不受影响。
type RedisRelay struct {
	nodeID  string
	channel string
	rdb     *redis.Client
	pubsub  *redis.PubSub
	stop    chan struct{}
}

// NewRedisRelay 连接并订阅；ping 失败立即报错（坏地址在启动期暴露，
// 不做静默半可用实例）。
func NewRedisRelay(addr, password, channel string, deliver func(userIds []int64, frame []byte)) (*RedisRelay, error) {
	if channel == "" {
		return nil, fmt.Errorf("relay channel required")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: password})
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping %s: %w", addr, err)
	}
	hostname, _ := os.Hostname()
	var randBuf [4]byte
	if _, err := rand.Read(randBuf[:]); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("generate node id: %w", err)
	}
	r := &RedisRelay{
		nodeID:  fmt.Sprintf("%s-%d-%x", hostname, os.Getpid(), randBuf[:]),
		channel: channel,
		rdb:     rdb,
		pubsub:  rdb.Subscribe(context.Background(), channel),
		stop:    make(chan struct{}),
	}
	// go-redis v9 订阅惰性建立：等订阅确认，保证本函数返回后发布的
	// 消息不会漏收。
	if _, err := r.pubsub.Receive(pingCtx); err != nil {
		_ = r.Close()
		return nil, fmt.Errorf("redis subscribe %s: %w", channel, err)
	}
	go r.readLoop(deliver)
	return r, nil
}

// readLoop 消费频道消息：剔除本实例回环后交本地投递器。
func (r *RedisRelay) readLoop(deliver func(userIds []int64, frame []byte)) {
	msgs := r.pubsub.Channel(redis.WithChannelSize(256))
	for {
		select {
		case <-r.stop:
			return
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			var env redisEnvelope
			if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
				log.Printf("relay: bad envelope: %v", err)
				continue
			}
			if env.From == r.nodeID {
				continue
			}
			deliver(env.UserIds, env.Frame)
		}
	}
}

// Broadcast 广播一帧到频道（fire-and-forget，失败仅记日志）。
func (r *RedisRelay) Broadcast(userIds []int64, frame []byte) {
	payload, err := json.Marshal(redisEnvelope{From: r.nodeID, UserIds: userIds, Frame: frame})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.rdb.Publish(ctx, r.channel, payload).Err(); err != nil {
		log.Printf("relay: publish: %v", err)
	}
}

// Close 停止读取并释放连接。
func (r *RedisRelay) Close() error {
	close(r.stop)
	return r.rdb.Close()
}
