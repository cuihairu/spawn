package model

import "errors"

var (
	ErrPostNotFound     = errors.New("post not found")
	ErrTopicNotFound    = errors.New("topic not found")
	ErrCommentNotFound  = errors.New("comment not found")
	ErrReportNotFound   = errors.New("report not found")
	ErrNotifNotFound    = errors.New("notification not found")
	ErrPermissionDenied = errors.New("permission denied")
	ErrDesignNotFound   = errors.New("design breakdown not found")
	// ErrDesignCommentNotFound 拆解评论不存在或非本人（不区分以不泄露存在性）
	ErrDesignCommentNotFound = errors.New("design comment not found")
)
