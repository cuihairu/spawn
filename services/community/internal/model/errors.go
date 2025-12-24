package model

import "errors"

var (
	ErrPostNotFound     = errors.New("post not found")
	ErrTopicNotFound    = errors.New("topic not found")
	ErrPermissionDenied = errors.New("permission denied")
)
