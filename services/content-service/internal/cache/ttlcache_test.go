package cache

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// fakeClock 可控时钟，避免真实 sleep。
type fakeClock struct{ t time.Time }

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}
}
func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestSetGetDelete(t *testing.T) {
	c := New[string, int](time.Minute, 8)

	if _, ok := c.Get("a"); ok {
		t.Fatal("empty cache must miss")
	}
	c.Set("a", 42)
	if v, ok := c.Get("a"); !ok || v != 42 {
		t.Fatalf("Get = %d, %v; want 42, true", v, ok)
	}

	c.Set("a", 43) // 覆盖
	if v, _ := c.Get("a"); v != 43 {
		t.Fatalf("overwrite failed: %d", v)
	}

	c.Delete("a")
	if _, ok := c.Get("a"); ok {
		t.Fatal("deleted key must miss")
	}
	c.Delete("never-existed") // 幂等
}

func TestTTLExpiry(t *testing.T) {
	clk := newFakeClock()
	c := New[string, string](time.Minute, 8)
	c.now = clk.now

	c.Set("k", "v")
	clk.advance(59 * time.Second)
	if _, ok := c.Get("k"); !ok {
		t.Fatal("must hit before TTL elapses")
	}
	clk.advance(2 * time.Second) // 共 61s > 60s
	if v, ok := c.Get("k"); ok {
		t.Fatalf("expired key hit: %q", v)
	}
	if c.Len() != 0 {
		t.Fatalf("expired entry must be evicted lazily, Len = %d", c.Len())
	}
}

func TestSetRefreshesTTL(t *testing.T) {
	clk := newFakeClock()
	c := New[string, int](time.Minute, 8)
	c.now = clk.now

	c.Set("k", 1)
	clk.advance(50 * time.Second)
	c.Set("k", 2)                 // 刷新过期时间
	clk.advance(50 * time.Second) // 距首次写入 100s，距刷新 50s
	if v, ok := c.Get("k"); !ok || v != 2 {
		t.Fatalf("refreshed entry Get = %d, %v; want 2, true", v, ok)
	}
}

func TestLRUEviction(t *testing.T) {
	clk := newFakeClock()
	c := New[string, int](time.Minute, 2)
	c.now = clk.now

	c.Set("a", 1)
	c.Set("b", 2)
	if _, ok := c.Get("a"); !ok { // a 变为最新
		t.Fatal("a must hit")
	}
	c.Set("c", 3) // 淘汰最久未用的 b
	if _, ok := c.Get("b"); ok {
		t.Fatal("b must be evicted (LRU)")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a must survive")
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatal("c must survive")
	}
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
}

func TestDisabledCache(t *testing.T) {
	c := New[string, int](0, 8) // ttl<=0 → 禁用
	c.Set("k", 1)
	if _, ok := c.Get("k"); ok {
		t.Fatal("disabled cache must not store")
	}
	if c.Len() != 0 {
		t.Fatalf("disabled cache Len = %d, want 0", c.Len())
	}
}

func TestDefaultCapacity(t *testing.T) {
	c := New[int, int](time.Minute, 0) // max<=0 → 默认 1024
	for i := 0; i < 2000; i++ {
		c.Set(i, i)
	}
	if c.Len() != 1024 {
		t.Fatalf("Len = %d, want 1024", c.Len())
	}
	if _, ok := c.Get(1999); !ok { // 最新键保留
		t.Fatal("newest key must survive")
	}
}

// TestConcurrentAccess 并发读写冒烟（-race 下运行）。
func TestConcurrentAccess(t *testing.T) {
	c := New[string, int](time.Minute, 64)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				k := "k" + strconv.Itoa(i%32)
				c.Set(k, i)
				c.Get(k)
				if i%7 == 0 {
					c.Delete(k)
				}
				_ = c.Len()
			}
		}(g)
	}
	wg.Wait()
}
