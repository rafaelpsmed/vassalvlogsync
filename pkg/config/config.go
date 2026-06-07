package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type GameConfig struct {
	GameID       string `json:"game_id"`
	InviteToken  string `json:"invite_token"`
	ClientToken  string `json:"client_token"`
	GameName     string `json:"game_name"`
	VassalModule string `json:"vassal_module"`
	PlayerName   string `json:"player_name"`
}

type ClientConfig struct {
	WatchDir     string                `json:"watch_dir"`
	ServerURL    string                `json:"server_url"`
	GameID       string                `json:"game_id"`
	InviteToken  string                `json:"invite_token"`
	ClientToken  string                `json:"client_token"`
	GameName     string                `json:"game_name"`
	VassalModule string                `json:"vassal_module"`
	PlayerName   string                `json:"player_name"`
	Games        map[string]GameConfig `json:"games"`
}

func (c *ClientConfig) ActiveGame() *GameConfig {
	if c.GameID == "" {
		return nil
	}
	return c.gameConfig(c.GameID)
}

func (c *ClientConfig) gameConfig(gameID string) *GameConfig {
	if gc, ok := c.Games[gameID]; ok {
		return &gc
	}
	return nil
}

func (c *ClientConfig) GameCount() int {
	return len(c.Games)
}

func (c *ClientConfig) GameList() []GameConfig {
	var list []GameConfig
	for _, g := range c.Games {
		list = append(list, g)
	}
	return list
}

func (c *ClientConfig) AddGame(gc GameConfig) {
	if c.Games == nil {
		c.Games = make(map[string]GameConfig)
	}
	c.Games[gc.GameID] = gc
	c.switchTo(gc)
}

func (c *ClientConfig) SwitchGame(gameID string) bool {
	gc, ok := c.Games[gameID]
	if !ok {
		return false
	}
	c.switchTo(gc)
	return true
}

func (c *ClientConfig) switchTo(gc GameConfig) {
	c.GameID = gc.GameID
	c.InviteToken = gc.InviteToken
	c.ClientToken = gc.ClientToken
	c.GameName = gc.GameName
	c.VassalModule = gc.VassalModule
	c.PlayerName = gc.PlayerName
}

func (c *ClientConfig) RemoveGame(gameID string) {
	delete(c.Games, gameID)
	if c.GameID == gameID {
		c.GameID = ""
		c.InviteToken = ""
		c.ClientToken = ""
		c.GameName = ""
		c.VassalModule = ""
		c.PlayerName = ""
	}
}

func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vassal-vlog-sync", "config.json"), nil
}

func LoadClient() (*ClientConfig, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ClientConfig{ServerURL: "http://localhost:8080"}, nil
		}
		return nil, err
	}
	var cfg ClientConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.ServerURL == "" {
		cfg.ServerURL = "http://localhost:8080"
	}
	if cfg.Games == nil {
		cfg.Games = make(map[string]GameConfig)
		if cfg.GameID != "" {
			cfg.Games[cfg.GameID] = GameConfig{
				GameID:       cfg.GameID,
				InviteToken:  cfg.InviteToken,
				ClientToken:  cfg.ClientToken,
				GameName:     cfg.GameName,
				VassalModule: cfg.VassalModule,
				PlayerName:   cfg.PlayerName,
			}
		}
	}
	return &cfg, nil
}

func SaveClient(cfg *ClientConfig) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func ExtractInviteToken(input string) string {
	input = trimSpace(input)
	for i := len(input) - 1; i >= 0; i-- {
		if input[i] == '/' {
			return input[i+1:]
		}
	}
	return input
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}
