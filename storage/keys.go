package storage

import (
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/player"
)

// All the Redis keys. Keys should be never concatenated at call sites.

// All keys live under this prefix.
const keyPrefix = "app:"

const (
	playerNS   = keyPrefix + "player:"
	boardNS    = keyPrefix + "board:"
	viewNS     = keyPrefix + "view:" // note: we usually use "projection" word
	adminNS    = keyPrefix + "admin:"
	ledgerNS   = keyPrefix + "ledger:"
	consumerNS = keyPrefix + "consumer:"
)

const (
	// ZSET registry: member=board_id, score=created_at unix
	boardIndexKey = boardNS + "index"
	// STREAM
	ledgerKey = ledgerNS + "events"
	// HASH {board_id}:{player_id}:{idempotency_key} -> "entry_id|op|amount"
	idempotencyHashKey = ledgerNS + "idempotency"
	// HASH client key -> "player_id|player_name"
	playerIdempotencyHashKey = playerNS + "idempotency"
)

const playerHistoryNS = viewNS + "player:history:"

func playerProfileKey(id player.ID) string { return playerNS + "profile:" + string(id) }

func boardProfileKey(id board.ID) string { return boardNS + "profile:" + string(id) }

// per-board ZSET projection
func leaderboardKey(id board.ID) string { return viewNS + "leaderboard:" + string(id) }

// per-board ZSET scratch: transient rebuild target for VerifyProjection
func boardVerifyKey(id board.ID) string { return adminNS + "temp:verify:" + string(id) }

// per-(player, board) ZSET projection (member=event id, score = millis * 4096 + seq)
func playerHistoryKey(playerId player.ID, boardId board.ID) string {
	return playerHistoryNS + string(playerId) + ":" + string(boardId)
}

// per-day ZSET projection: member=player_id, score=event count. date is UTC YYYY-MM-DD.
func activityDailyKey(date string) string { return viewNS + "activity:daily:" + date }

// last processed ledger stream id for a named async consumer.
func consumerCursorKey(name string) string { return consumerNS + name + ":cursor" }
