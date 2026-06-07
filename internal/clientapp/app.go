package clientapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/rafael/vassal-vlog-sync/internal/syncengine"
	"github.com/rafael/vassal-vlog-sync/pkg/config"
	"github.com/rafael/vassal-vlog-sync/pkg/models"
)

type App struct {
	mu     sync.RWMutex
	cfg    *config.ClientConfig
	engine *syncengine.Engine
	state  *models.GameState
	client *http.Client
}

func New() (*App, error) {
	cfg, err := config.LoadClient()
	if err != nil {
		return nil, err
	}
	a := &App{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
	a.engine = syncengine.New(cfg, a.onStateChange)
	return a, nil
}

func (a *App) Config() *config.ClientConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (a *App) State() *models.GameState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

func (a *App) onStateChange(state *models.GameState) {
	a.mu.Lock()
	a.state = state
	a.mu.Unlock()
}

func (a *App) SaveConfig(cfg *config.ClientConfig) error {
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	a.engine.SetConfig(cfg)
	return config.SaveClient(cfg)
}

func (a *App) CreateGame(name, vassalModule, hostName, hostEmail string) (*models.CreateGameResponse, error) {
	cfg := a.Config()
	req := models.CreateGameRequest{
		Name:         name,
		VassalModule: vassalModule,
		HostName:     hostName,
		HostEmail:    hostEmail,
	}
	var resp models.CreateGameResponse
	if err := a.postJSON(cfg.ServerURL+"/games", req, &resp); err != nil {
		return nil, err
	}

	cfg.AddGame(config.GameConfig{
		GameID:       resp.GameID,
		InviteToken:  resp.InviteToken,
		ClientToken:  resp.ClientToken,
		GameName:     resp.GameName,
		VassalModule: resp.VassalModule,
		PlayerName:   hostName,
	})
	if err := a.SaveConfig(cfg); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (a *App) JoinGame(inviteInput, name, email string) (*models.JoinResponse, error) {
	cfg := a.Config()
	req := models.JoinRequest{
		InviteToken: config.ExtractInviteToken(inviteInput),
		Name:        name,
		Email:       email,
	}
	var resp models.JoinResponse
	if err := a.postJSON(cfg.ServerURL+"/join", req, &resp); err != nil {
		return nil, err
	}

	cfg.AddGame(config.GameConfig{
		GameID:       resp.GameID,
		InviteToken:  config.ExtractInviteToken(inviteInput),
		ClientToken:  resp.ClientToken,
		GameName:     resp.GameName,
		VassalModule: resp.VassalModule,
		PlayerName:   name,
	})
	if err := a.SaveConfig(cfg); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (a *App) StartGame() error {
	cfg := a.Config()
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/games/%s/start", cfg.ServerURL, cfg.GameID), nil)
	req.Header.Set("Authorization", "Bearer "+cfg.ClientToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("start %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (a *App) LeaveGame() error {
	cfg := a.Config()
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/games/%s/leave", cfg.ServerURL, cfg.GameID), nil)
	req.Header.Set("Authorization", "Bearer "+cfg.ClientToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("leave %d: %s", resp.StatusCode, string(b))
	}

	cfg.RemoveGame(cfg.GameID)
	if err := a.SaveConfig(cfg); err != nil {
		return err
	}
	return nil
}

func (a *App) SwitchGame(gameID string) ([]config.GameConfig, error) {
	cfg := a.Config()
	if !cfg.SwitchGame(gameID) {
		return nil, fmt.Errorf("partida não encontrada")
	}
	if err := a.SaveConfig(cfg); err != nil {
		return nil, err
	}
	state, _ := a.RefreshState()
	if state != nil {
		a.onStateChange(state)
	}
	return cfg.GameList(), nil
}

func (a *App) GameList() []config.GameConfig {
	return a.Config().GameList()
}

func (a *App) GameCount() int {
	return a.Config().GameCount()
}

func (a *App) SetTurnOrder(gameID string, playerIDs []string) error {
	cfg := a.Config()
	req := models.TurnOrderRequest{PlayerIDs: playerIDs}
	return a.patchJSON(fmt.Sprintf("%s/games/%s/turn-order", cfg.ServerURL, gameID), req)
}

func (a *App) RefreshState() (*models.GameState, error) {
	cfg := a.Config()
	if cfg.GameID == "" {
		return nil, fmt.Errorf("nenhuma partida associada")
	}
	state, err := a.fetchGameState(cfg.ServerURL, cfg.GameID, cfg.ClientToken)
	if err != nil {
		return nil, err
	}
	a.onStateChange(state)
	return state, nil
}

func (a *App) RefreshAllStates() map[string]*models.GameState {
	cfg := a.Config()
	result := make(map[string]*models.GameState)
	for gameID, gc := range cfg.Games {
		state, err := a.fetchGameState(cfg.ServerURL, gameID, gc.ClientToken)
		if err != nil {
			continue
		}
		result[gameID] = state
		if gameID == cfg.GameID {
			a.onStateChange(state)
		}
	}
	return result
}

func (a *App) fetchGameState(serverURL, gameID, clientToken string) (*models.GameState, error) {
	u := fmt.Sprintf("%s/games/%s/state?token=%s", serverURL, gameID, url.QueryEscape(clientToken))
	resp, err := a.client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var state models.GameState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (a *App) RunSync() error {
	return a.engine.Run()
}

func (a *App) SetWatchDir(dir string) error {
	cfg := a.Config()
	cfg.WatchDir = dir
	return a.SaveConfig(cfg)
}

func (a *App) SetServerURL(url string) error {
	cfg := a.Config()
	cfg.ServerURL = url
	return a.SaveConfig(cfg)
}

func (a *App) postJSON(url string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := a.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request %d: %s", resp.StatusCode, string(b))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (a *App) patchJSON(url string, body any) error {
	cfg := a.Config()
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.ClientToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
