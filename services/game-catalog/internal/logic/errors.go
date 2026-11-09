package logic

import (
	"context"
	"errors"
	"net/http"
)

type apiError struct {
	code    int
	message string
}

func (e *apiError) Error() string {
	return e.message
}

func (e *apiError) StatusCode() int {
	return e.code
}

func newAPIError(code int, message string) error {
	return &apiError{code: code, message: message}
}

// NewAPIError 导出给 handler 层构造带真实状态码的业务错误：
// 用于 multipart 解析等不经 httpx.Parse 的入口（GP-105 包体上传）。
func NewAPIError(code int, message string) error {
	return newAPIError(code, message)
}

// ErrorHandler 将 logic 层 apiError 的状态码渲染为真实 HTTP 状态；
// 供 main 通过 httpx.SetErrorHandlerCtx 注册，使 404/500 语义真正生效。
// 非 apiError（参数解析等）保持 go-zero 默认的 400 语义。
func ErrorHandler(_ context.Context, err error) (int, any) {
	var apiErr *apiError
	if errors.As(err, &apiErr) && apiErr != nil {
		return apiErr.code, apiErr.message
	}
	return http.StatusBadRequest, err.Error()
}
