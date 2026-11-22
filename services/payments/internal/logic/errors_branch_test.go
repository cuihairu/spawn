package logic

// errors.go 剩余分支：ErrorHandler 对非 apiError 兜底（go-zero 默认 400 + 原文），
// 以及 NewAPIError 导出构造。

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestErrorHandler_NonAPIError(t *testing.T) {
	code, msg := ErrorHandler(context.Background(), errors.New("参数解析失败"))
	if code != http.StatusBadRequest || msg != "参数解析失败" {
		t.Fatalf("ErrorHandler(non-apiError) = (%d,%v), want (400,原文)", code, msg)
	}
}
