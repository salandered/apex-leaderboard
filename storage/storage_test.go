package storage

import (
	"context"
	"testing"
	"time"
)

func TestRedisConnection(t *testing.T) {
	rs := NewStorage()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rs.client.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not reachable at %s: %v", rs.client.Options().Addr, err)
	}
}
