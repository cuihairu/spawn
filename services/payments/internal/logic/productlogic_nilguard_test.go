package logic

// productlogic.go 剩余补漏分支：toTypesProduct 的 nil 守卫（仅白盒调用可达——
// 上层只传非 nil 的 *model.Product，正常流程不会走 nil 分支）。

import "testing"

func TestToTypesProduct_NilGuard(t *testing.T) {
	p := toTypesProduct(nil)
	if p == nil {
		t.Fatal("toTypesProduct(nil) = nil, want empty Product")
	}
	if p.ProductId != "" || p.AmountCents != 0 || p.Enabled {
		t.Fatalf("toTypesProduct(nil) = %+v, want zero-value Product", p)
	}
}
