package courier

// client.go 剩余补漏分支：ForwardCallback 与 call 的 http.NewRequest 失败臂
// （baseURL 非法，url.Parse 失败）。
//
// 登记不可达（不测）：161.4 json.Marshal（固定类型结构体/map 恒可序列化）
// 失败臂；141.3 / 186.3 io.ReadAll（io.LimitReader 不产生读错误）失败臂。

import (
	"testing"
	"time"
)

func TestClientNewRequestArms(t *testing.T) {
	// baseURL 非法 → ForwardCallback 的 NewRequest 失败臂（130）
	c := NewClient("://bad", time.Second)
	if err := c.ForwardCallback([]byte(`{}`), "x"); err == nil {
		t.Fatal("forward bad url = nil, want error")
	}
	// 同一非法 baseURL → call 的 NewRequest 失败臂（167）
	if _, err := c.ListSkus("g", "prod", "Bearer t"); err == nil {
		t.Fatal("call bad url = nil, want error")
	}
}
