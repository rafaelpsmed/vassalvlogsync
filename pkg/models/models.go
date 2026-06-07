package models

import "time"

type GameStatus string

const (
	GameStatusLobby    GameStatus = "lobby"
	GameStatusActive   GameStatus = "active"
	GameStatusFinished GameStatus = "finished"
)

type Game struct {
	ID               string     `json:"id"`
	InviteToken      string     `json:"invite_token"`
	Name             string     `json:"name"`
	VassalModule     string     `json:"vassal_module"`
	Status           GameStatus `json:"status"`
	CurrentTurnIndex int        `json:"current_turn_index"`
	HostPlayerID     string     `json:"host_player_id"`
	CreatedAt        time.Time  `json:"created_at"`
}

type Player struct {
	ID          string `json:"id"`
	GameID      string `json:"game_id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	ClientToken string `json:"client_token,omitempty"`
	TurnOrder   int    `json:"turn_order"`
}

type Turn struct {
	ID         string    `json:"id"`
	GameID     string    `json:"game_id"`
	PlayerID   string    `json:"player_id"`
	VlogPath   string    `json:"vlog_path"`
	DateSaved  string    `json:"date_saved"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type GameState struct {
	Game          Game     `json:"game"`
	Players       []Player `json:"players"`
	CurrentPlayer *Player  `json:"current_player,omitempty"`
	LastDateSaved string   `json:"last_date_saved,omitempty"`
	YourTurn      bool     `json:"your_turn"`
	YourTurnOrder int      `json:"your_turn_order"`
}

type CreateGameRequest struct {
	Name         string `json:"name"`
	VassalModule string `json:"vassal_module"`
	HostName     string `json:"host_name"`
	HostEmail    string `json:"host_email"`
}

type CreateGameResponse struct {
	GameID       string `json:"game_id"`
	InviteToken  string `json:"invite_token"`
	InviteURL    string `json:"invite_url"`
	VassalModule string `json:"vassal_module"`
	GameName     string `json:"game_name"`
	ClientToken  string `json:"client_token"`
	TurnOrder    int    `json:"turn_order"`
}

type JoinRequest struct {
	InviteToken string `json:"invite_token"`
	Name        string `json:"name"`
	Email       string `json:"email"`
}

type JoinResponse struct {
	GameID       string `json:"game_id"`
	ClientToken  string `json:"client_token"`
	GameName     string `json:"game_name"`
	VassalModule string `json:"vassal_module"`
	TurnOrder    int    `json:"turn_order"`
}

type TurnOrderRequest struct {
	PlayerIDs []string `json:"player_ids"`
}

type WSEvent struct {
	Type          string `json:"type"`
	GameID        string `json:"game_id"`
	YourTurn      bool   `json:"your_turn,omitempty"`
	CurrentPlayer string `json:"current_player,omitempty"`
	PlayerName    string `json:"player_name,omitempty"`
	DateSaved     string `json:"date_saved,omitempty"`
}
