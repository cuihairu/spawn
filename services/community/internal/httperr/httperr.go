package httperr

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/tappi/tappi/services/community/internal/model"
)

type Error struct {
	Status  int
	Code    int64
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type Response struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

func New(status int, code int64, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func BadRequest(message string) *Error {
	return New(http.StatusBadRequest, http.StatusBadRequest, message)
}

func Unauthorized(message string) *Error {
	return New(http.StatusUnauthorized, http.StatusUnauthorized, message)
}

func Forbidden(message string) *Error {
	return New(http.StatusForbidden, http.StatusForbidden, message)
}

func NotFound(message string) *Error {
	return New(http.StatusNotFound, http.StatusNotFound, message)
}

func Internal(message string) *Error {
	if strings.TrimSpace(message) == "" {
		message = "internal server error"
	}
	return New(http.StatusInternalServerError, http.StatusInternalServerError, message)
}

func ErrorHandler(_ context.Context, err error) (int, any) {
	var appErr *Error
	if errors.As(err, &appErr) && appErr != nil {
		return appErr.Status, Response{Code: appErr.Code, Message: appErr.Message}
	}

	switch {
	case errors.Is(err, model.ErrPostNotFound), errors.Is(err, model.ErrTopicNotFound):
		return http.StatusNotFound, Response{Code: http.StatusNotFound, Message: err.Error()}
	case errors.Is(err, model.ErrPermissionDenied):
		return http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "permission denied"}
	}

	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case lower == "unauthorized":
		return http.StatusUnauthorized, Response{Code: http.StatusUnauthorized, Message: "unauthorized"}
	case strings.Contains(lower, "permission denied"):
		return http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "permission denied"}
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "internal server error"}
	}

	return http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error()}
}
