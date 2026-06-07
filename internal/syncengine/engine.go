package syncengine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rafael/vassal-vlog-sync/internal/notify"
	"github.com/rafael/vassal-vlog-sync/internal/vlog"
	"github.com/rafael/vassal-vlog-sync/internal/watcher"
	"github.com/rafael/vassal-vlog-sync/pkg/config"
	"github.com/rafael/vassal-vlog-sync/pkg/models"
)

type StateCallback func(state *models.GameState)

type Engine struct {
	cfg        *config.ClientConfig
	watcher    *watcher.Watcher
	onState    StateCallback
	mu         sync.Mutex
	lastUpload string
	lastSync   string
	httpClient *http.Client
}

func New(cfg *config.ClientConfig, onState StateCallback) *Engine {
	e := &Engine{
		cfg:        cfg,
		onState:    onState,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
	e.watcher = watcher.New(cfg.WatchDir, e.onVlogChange)
	return e
}

func (e *Engine) SetConfig(cfg *config.ClientConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg = cfg
	if e.watcher != nil && cfg.WatchDir != "" {
		e.watcher = watcher.New(cfg.WatchDir, e.onVlogChange)
	}
}

func (e *Engine) Config() *config.ClientConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

func (e *Engine) Run() error {
	go e.runWebSocket()
	go e.pollState()

	for {
		cfg := e.Config()
		if cfg.WatchDir != "" && cfg.GameID != "" && cfg.ClientToken != "" {
			e.watcher = watcher.New(cfg.WatchDir, e.onVlogChange)
			return e.watcher.Run()
		}
		time.Sleep(2 * time.Second)
	}
}

func (e *Engine) onVlogChange(path, dateSaved string) {
	cfg := e.Config()
	if cfg.GameID == "" {
		return
	}

	state, err := e.fetchState(cfg)
	if err != nil {
		log.Printf("state error: %v", err)
		return
	}
	if !state.YourTurn {
		log.Printf("save detectado mas não é seu turno (%s)", filepath.Base(path))
		return
	}

	e.mu.Lock()
	if e.lastUpload == dateSaved || e.lastSync == dateSaved {
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()

	if err := e.upload(cfg, path, dateSaved); err != nil {
		log.Printf("upload error: %v", err)
		return
	}

	e.mu.Lock()
	e.lastUpload = dateSaved
	e.mu.Unlock()

	log.Printf("upload concluído: %s (dateSaved=%s)", filepath.Base(path), dateSaved)

	state, _ = e.fetchState(cfg)
	if state != nil && e.onState != nil {
		e.onState(state)
	}
}

func (e *Engine) upload(cfg *config.ClientConfig, path, dateSaved string) error {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*attempt) * time.Second)
		}
		if err := e.uploadOnce(cfg, path, dateSaved); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func (e *Engine) uploadOnce(cfg *config.ClientConfig, path, dateSaved string) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("vlog", filepath.Base(path))
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	_ = writer.WriteField("date_saved", dateSaved)
	writer.Close()

	reqURL := fmt.Sprintf("%s/games/%s/upload", cfg.ServerURL, cfg.GameID)
	req, err := http.NewRequest(http.MethodPost, reqURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+cfg.ClientToken)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (e *Engine) DownloadLatest() error {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*attempt) * time.Second)
		}
		if err := e.downloadOnce(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func (e *Engine) downloadOnce() error {
	cfg := e.Config()
	if cfg.GameID == "" || cfg.WatchDir == "" {
		return fmt.Errorf("config incompleta")
	}

	reqURL := fmt.Sprintf("%s/games/%s/download", cfg.ServerURL, cfg.GameID)
	resp, err := e.httpClient.Get(reqURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download %d: %s", resp.StatusCode, string(b))
	}

	destName := fmt.Sprintf("%s_sync.vlog", cfg.GameName)
	if destName == "_sync.vlog" || cfg.GameName == "" {
		destName = "game_sync.vlog"
	}
	destName = sanitizeFilename(destName)
	destPath := filepath.Join(cfg.WatchDir, destName)

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return err
	}

	dateSaved, err := vlog.ReadDateSaved(destPath)
	if err != nil {
		log.Printf("aviso: não leu dateSaved após download: %v", err)
	} else {
		e.mu.Lock()
		e.lastSync = dateSaved
		e.mu.Unlock()
		e.watcher.IgnoreDateSaved(dateSaved)
	}

	log.Printf("download salvo em %s", destPath)
	return nil
}

func (e *Engine) fetchState(cfg *config.ClientConfig) (*models.GameState, error) {
	u := fmt.Sprintf("%s/games/%s/state?token=%s", cfg.ServerURL, cfg.GameID, url.QueryEscape(cfg.ClientToken))
	resp, err := e.httpClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("state %d: %s", resp.StatusCode, string(b))
	}
	var state models.GameState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (e *Engine) pollState() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		cfg := e.Config()
		if cfg.GameID == "" {
			continue
		}
		state, err := e.fetchState(cfg)
		if err != nil {
			continue
		}
		if e.onState != nil {
			e.onState(state)
		}
	}
}

func (e *Engine) runWebSocket() {
	const (
		minBackoff   = 1 * time.Second
		maxBackoff   = 2 * time.Minute
		pingPeriod   = 30 * time.Second
		writeTimeout = 10 * time.Second
		pongTimeout  = 60 * time.Second
	)

	backoff := minBackoff
	retryDelay := func() time.Duration {
		return minBackoff
	}

	for {
		cfg := e.Config()
		if cfg.GameID == "" || cfg.ClientToken == "" {
			time.Sleep(2 * time.Second)
			backoff = minBackoff
			continue
		}

		delay := retryDelay()
		log.Printf("ws: conectando em %v...", delay.Round(time.Second))
		time.Sleep(delay)

		wsURL := toWS(cfg.ServerURL) + "/games/" + cfg.GameID + "/events?token=" + url.QueryEscape(cfg.ClientToken)
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			log.Printf("ws connect error: %v", err)
			backoff = nextBackoff(backoff, maxBackoff)
			retryDelay = jitteredDelay(backoff)
			continue
		}

		backoff = minBackoff
		retryDelay = func() time.Duration { return minBackoff }
		log.Printf("ws: conectado ao servidor (game=%s)", cfg.GameID)

		e.serveWS(conn, pingPeriod, writeTimeout, pongTimeout)
		log.Printf("ws: desconectado (game=%s)", cfg.GameID)

		backoff = nextBackoff(backoff, maxBackoff)
		retryDelay = jitteredDelay(backoff)
	}
}

func (e *Engine) serveWS(conn *websocket.Conn, pingPeriod, writeTimeout, pongTimeout time.Duration) {
	defer conn.Close()

	pongCh := make(chan struct{}, 1)
	conn.SetPongHandler(func(string) error {
		select {
		case pongCh <- struct{}{}:
		default:
		}
		return nil
	})

	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(writeTimeout))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	for {
		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var event models.WSEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			continue
		}
		e.handleWSEvent(event)
	}
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

func jitteredDelay(base time.Duration) func() time.Duration {
	return func() time.Duration {
		jitter := time.Duration(rand.Int64N(int64(base)))
		return base/2 + jitter
	}
}

func (e *Engine) handleWSEvent(event models.WSEvent) {
	switch event.Type {
	case "turn_changed":
		if event.YourTurn {
			if err := e.DownloadLatest(); err != nil {
				log.Printf("download error: %v", err)
			}
			cfg := e.Config()
			title := "Vassal vLog Sync"
			message := fmt.Sprintf("É sua vez na partida %s!", cfg.GameName)
			_ = notify.Notify(title, message)
		}
		cfg := e.Config()
		state, err := e.fetchState(cfg)
		if err == nil && e.onState != nil {
			e.onState(state)
		}
	case "player_joined":
		log.Printf("jogador entrou: %s", event.PlayerName)
	case "player_left":
		log.Printf("jogador saiu: %s", event.PlayerName)
		cfg := e.Config()
		state, err := e.fetchState(cfg)
		if err == nil && e.onState != nil {
			e.onState(state)
		}
	case "game_ended":
		log.Printf("partida encerrada: %s", event.GameID)
		_ = notify.Notify("Vassal vLog Sync", "A partida foi encerrada.")
	}
}

func toWS(serverURL string) string {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "ws://localhost:8080"
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		u.Scheme = "ws"
	}
	return u.String()
}

func sanitizeFilename(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			out = append(out, '_')
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}
