// Package cache 提供进程内 TTL+LRU 键值缓存（user-service 模型层读缓存）。
//
// 选型背景见 docs/development-guide.md「数据库集成 → 设计决策」：
// 当前无多实例部署且 CI 无 Redis 服务，进程内缓存语义正确、零外部依赖；
// 需跨进程失效时在同一接入点替换为 go-zero cache.Cache（Redis）。
package cache

import (
	"container/list"
	"sync"
	"time"
)

// Cache 进程内键值缓存：TTL 过期 + LRU 容量上限。
// 必须经 New 构造（零值不可用）。ttl<=0 表示禁用：Get 恒未命中，Set 为 no-op。
// 时钟经 now 字段注入，测试可替换以避免真实 sleep。
type Cache[K comparable, V any] struct {
	ttl time.Duration
	max int
	now func() time.Time

	mu    sync.Mutex
	ll    *list.List // 前端最新
	items map[K]*list.Element
}

type entry[K comparable, V any] struct {
	key       K
	val       V
	expiresAt time.Time
}

// New 构造缓存。ttl<=0 返回禁用状态的缓存；max<=0 取默认 1024。
func New[K comparable, V any](ttl time.Duration, max int) *Cache[K, V] {
	if max <= 0 {
		max = 1024
	}
	return &Cache[K, V]{
		ttl:   ttl,
		max:   max,
		now:   time.Now,
		ll:    list.New(),
		items: make(map[K]*list.Element),
	}
}

// Get 取值；命中时刷新其在 LRU 中的位置。过期项就地清除。
func (c *Cache[K, V]) Get(key K) (V, bool) {
	var zero V
	if c.ttl <= 0 {
		return zero, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return zero, false
	}
	ent := el.Value.(*entry[K, V])
	if c.now().After(ent.expiresAt) {
		c.ll.Remove(el)
		delete(c.items, key)
		return zero, false
	}
	c.ll.MoveToFront(el)
	return ent.val, true
}

// Set 写入（或刷新）键值并重置过期时间；超出容量时淘汰最久未用项。
func (c *Cache[K, V]) Set(key K, val V) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	if el, ok := c.items[key]; ok {
		ent := el.Value.(*entry[K, V])
		ent.val = val
		ent.expiresAt = now.Add(c.ttl)
		c.ll.MoveToFront(el)
		return
	}
	c.items[key] = c.ll.PushFront(&entry[K, V]{key: key, val: val, expiresAt: now.Add(c.ttl)})
	for c.ll.Len() > c.max {
		back := c.ll.Back()
		c.ll.Remove(back)
		delete(c.items, back.Value.(*entry[K, V]).key)
	}
}

// Delete 删除键（模型层写路径的失效点）。
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.Remove(el)
		delete(c.items, key)
	}
}

// Len 返回当前存活条目数（含尚未按 Get 清理的过期项）。
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
