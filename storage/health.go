package storage

import (
	"context"
	"fmt"
)

func (rs *redisStorage) Ping(ctx context.Context) error {
	if err := rs.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("storage ping: %w", err)
	}
	return nil
}
