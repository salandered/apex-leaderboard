package storage

import (
	"context"
	"fmt"
)

type ActivityEntry struct {
	PlayerID string
	Count    int64
}

// An unknown or empty date reads as an empty list, not an error.
func (rs *redisStorage) ListDailyActivity(
	ctx context.Context, date string, limit int64,
) ([]ActivityEntry, error) {
	zSetMembers, err := rs.client.ZRevRangeWithScores(ctx, activityDailyKey(date), 0, limit-1).Result()
	if err != nil {
		return nil, fmt.Errorf("storage list daily activity: %w", err)
	}
	out := make([]ActivityEntry, 0, len(zSetMembers))
	for _, member := range zSetMembers {
		out = append(out, ActivityEntry{
			PlayerID: member.Member.(string),
			Count:    int64(member.Score),
		})
	}
	return out, nil
}
