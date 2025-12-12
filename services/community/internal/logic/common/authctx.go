package common

import (
	"context"
	"errors"
)

func UserFromContext(ctx context.Context) (userId int64, username string, err error) {
	raw := ctx.Value("user_id")
	id, ok := raw.(int64)
	if !ok || id <= 0 {
		return 0, "", errors.New("unauthorized")
	}

	rawName := ctx.Value("username")
	if name, ok := rawName.(string); ok {
		username = name
	}

	return id, username, nil
}
