package models

import playerid "github.com/salandered/apex/player_id"

type PlayerData struct {
	PlayerId    playerid.PlayerId
	PlayerName  string
	PlayerScore float64
}
