package model

import "errors"

var (
	ErrPostNotFound     = errors.New("post not found")
	ErrTopicNotFound    = errors.New("topic not found")
	ErrCommentNotFound  = errors.New("comment not found")
	ErrReportNotFound   = errors.New("report not found")
	ErrPermissionDenied = errors.New("permission denied")
)
