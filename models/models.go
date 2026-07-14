package models

import playerid "github.com/salandered/apex/player_id"

// TODO: DateAdded and other
type Profile struct {
	PlayerId   playerid.PlayerId
	PlayerName string
}

// ScoreEntry is a ranked leaderboard row for future reads like Top-N
type ScoreEntry struct {
	PlayerId   playerid.PlayerId
	PlayerName string
	Score      float64
	Rank       int64
}
