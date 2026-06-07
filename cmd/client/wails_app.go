package main

import (
	"context"
	"fmt"

	"github.com/rafael/vassal-vlog-sync/internal/clientapp"
	"github.com/rafael/vassal-vlog-sync/pkg/config"
	"github.com/rafael/vassal-vlog-sync/pkg/models"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type WailsApp struct {
	ctx  context.Context
	core *clientapp.App
}

func NewWailsApp(core *clientapp.App) *WailsApp {
	return &WailsApp{core: core}
}

func (a *WailsApp) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *WailsApp) GetConfig() *config.ClientConfig {
	return a.core.Config()
}

func (a *WailsApp) GetState() *models.GameState {
	return a.core.State()
}

func (a *WailsApp) SaveSettings(watchDir, serverURL string) error {
	cfg := a.core.Config()
	if watchDir != "" {
		cfg.WatchDir = watchDir
	}
	if serverURL != "" {
		cfg.ServerURL = serverURL
	}
	return a.core.SaveConfig(cfg)
}

func (a *WailsApp) SelectWatchDir() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Selecione a pasta do Vassal",
	})
	if err != nil {
		return "", err
	}
	if dir != "" {
		if err := a.core.SetWatchDir(dir); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func (a *WailsApp) CreateGame(name, vassalModule, hostName, hostEmail string) (*models.CreateGameResponse, error) {
	return a.core.CreateGame(name, vassalModule, hostName, hostEmail)
}

func (a *WailsApp) JoinGame(inviteInput, name, email string) (*models.JoinResponse, error) {
	return a.core.JoinGame(inviteInput, name, email)
}

func (a *WailsApp) StartGame() error {
	return a.core.StartGame()
}

func (a *WailsApp) LeaveGame() error {
	return a.core.LeaveGame()
}

func (a *WailsApp) SwitchGame(gameID string) ([]config.GameConfig, error) {
	return a.core.SwitchGame(gameID)
}

func (a *WailsApp) GameList() []config.GameConfig {
	return a.core.GameList()
}

func (a *WailsApp) SetTurnOrder(playerIDs []string) error {
	cfg := a.core.Config()
	return a.core.SetTurnOrder(cfg.GameID, playerIDs)
}

func (a *WailsApp) RefreshState() (*models.GameState, error) {
	return a.core.RefreshState()
}

func (a *WailsApp) RefreshAllStates() map[string]*models.GameState {
	return a.core.RefreshAllStates()
}

func (a *WailsApp) CopyToClipboard(text string) error {
	runtime.ClipboardSetText(a.ctx, text)
	return nil
}

func (a *WailsApp) StatusText() string {
	state := a.core.State()
	cfg := a.core.Config()
	if cfg.GameID == "" {
		return "Nenhuma partida associada"
	}
	if state == nil {
		return fmt.Sprintf("Partida: %s — módulo: %s", cfg.GameName, cfg.VassalModule)
	}
	if state.YourTurn {
		return fmt.Sprintf("Sua vez! (%s)", cfg.GameName)
	}
	if state.CurrentPlayer != nil {
		return fmt.Sprintf("Aguardando %s — %s", state.CurrentPlayer.Name, cfg.GameName)
	}
	return fmt.Sprintf("Lobby — %s (%d jogadores)", cfg.GameName, len(state.Players))
}

func (a *WailsApp) ShowWindow() {
	runtime.WindowShow(a.ctx)
}

func (a *WailsApp) HideWindow() {
	runtime.WindowHide(a.ctx)
}
