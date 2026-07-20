package storage

import (
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/player"
)

// The whole Redis keyspace of the service. Keys are never concatenated at call sites.

// All keys live under this prefix.
const keyPrefix = "app:"

const (
	playerNS = keyPrefix + "player:"
	boardNS  = keyPrefix + "board:"
	viewNS   = keyPrefix + "view:" // note: we usually use "projection" word
	adminNS  = keyPrefix + "admin:"
	ledgerNS = keyPrefix + "ledger:"
)

const (
	boardIndexKey            = boardNS + "index"        // ZSET registry: member=board_id, score=created_at unix
	ledgerKey                = ledgerNS + "events"      // STREAM
	idempotencyHashKey       = ledgerNS + "idempotency" // HASH {board_id}:{player_id}:{idempotency_key} -> "entry_id|op|amount"
	playerIdempotencyHashKey = playerNS + "idempotency" // HASH client key -> "player_id|player_name"
)

func playerProfileKey(id player.ID) string { return playerNS + "profile:" + string(id) }

func boardProfileKey(id board.ID) string { return boardNS + "profile:" + string(id) }

// per-board ZSET projection
func leaderboardKey(id board.ID) string { return viewNS + "leaderboard:" + string(id) }

// per-board ZSET scratch: transient rebuild target for VerifyProjection
func boardVerifyKey(id board.ID) string { return adminNS + "temp:verify:" + string(id) }
