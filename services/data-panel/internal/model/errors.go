package model

import "errors"

// 哨兵错误：HTTP 状态映射由 httperr.ErrorHandler 统一负责，
// logic/model 层只返回语义（与 community 服务同款分工）。
var ErrStatNotFound = errors.New("stat not found")
