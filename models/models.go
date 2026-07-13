package models

import playerid "github.com/salandered/apex/player_id"


// type PlayerScore struct {
// 	PlayerId    playerid.PlayerId
// 	PlayerScore float64
// }

type PlayerData struct {
	PlayerId    playerid.PlayerId
	PlayerName  string
	PlayerScore float64
}
