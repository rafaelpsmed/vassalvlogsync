package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/rafael/vassal-vlog-sync/internal/ws"
	"github.com/rafael/vassal-vlog-sync/pkg/config"
	"github.com/rafael/vassal-vlog-sync/pkg/models"
)

type Server struct {
	store       *Store
	hub         *ws.Hub
	mailer      *Mailer
	mux         *http.ServeMux
	rateLimiter RateLimiter
}

func NewServer(store *Store, hub *ws.Hub, mailer *Mailer) *Server {
	s := &Server{
		store:       store,
		hub:         hub,
		mailer:      mailer,
		mux:         http.NewServeMux(),
		rateLimiter: NewRateLimiterFromEnv(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.rateLimiter.Middleware(s.cors(s.mux))
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /games", s.handleCreateGame)
	s.mux.HandleFunc("POST /join", s.handleJoin)
	s.mux.HandleFunc("PATCH /games/{id}/turn-order", s.handleTurnOrder)
	s.mux.HandleFunc("POST /games/{id}/start", s.handleStart)
	s.mux.HandleFunc("POST /games/{id}/leave", s.handleLeave)
	s.mux.HandleFunc("POST /games/{id}/upload", s.handleUpload)
	s.mux.HandleFunc("GET /games/{id}/state", s.handleState)
	s.mux.HandleFunc("GET /games/{id}/download", s.handleDownload)
	s.mux.HandleFunc("GET /games/{id}/events", s.handleWS)
	s.mux.HandleFunc("GET /join/{token}", s.handleJoinPage)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	hs := map[string]string{
		"status":  "ok",
		"go":      runtime.Version(),
		"version": "0.1.0",
	}
	if err := s.store.db.PingContext(r.Context()); err != nil {
		hs["db"] = "error: " + err.Error()
		hs["status"] = "degraded"
	} else {
		hs["db"] = "ok"
	}
	if s.store.blob != nil {
		hs["storage"] = "ok"
	} else {
		hs["storage"] = "unavailable"
		hs["status"] = "degraded"
	}
	writeJSON(w, http.StatusOK, hs)
}

func (s *Server) handleCreateGame(w http.ResponseWriter, r *http.Request) {
	var req models.CreateGameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.VassalModule == "" || req.HostName == "" {
		http.Error(w, "name, vassal_module e host_name são obrigatórios", http.StatusBadRequest)
		return
	}
	resp, err := s.store.CreateGame(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	var req models.JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}
	req.InviteToken = config.ExtractInviteToken(req.InviteToken)
	if req.InviteToken == "" || req.Name == "" {
		http.Error(w, "invite_token e name são obrigatórios", http.StatusBadRequest)
		return
	}
	resp, err := s.store.JoinGame(r.Context(), req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrGameNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, ErrGameAlreadyStarted) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	s.hub.BroadcastPlayerJoined(resp.GameID, req.Name)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleTurnOrder(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("id")
	token := r.Header.Get("Authorization")
	token = strings.TrimPrefix(token, "Bearer ")

	var req models.TurnOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}
	if err := s.store.SetTurnOrder(r.Context(), gameID, token, req.PlayerIDs); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidToken) {
			status = http.StatusUnauthorized
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("id")
	token := r.Header.Get("Authorization")
	token = strings.TrimPrefix(token, "Bearer ")

	if err := s.store.StartGame(r.Context(), gameID, token); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidToken) {
			status = http.StatusUnauthorized
		} else if errors.Is(err, ErrGameAlreadyStarted) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) handleLeave(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("id")
	token := r.Header.Get("Authorization")
	token = strings.TrimPrefix(token, "Bearer ")

	player, game, err := s.store.LeaveGame(r.Context(), gameID, token)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidToken) {
			status = http.StatusUnauthorized
		}
		http.Error(w, err.Error(), status)
		return
	}

	s.hub.BroadcastGame(gameID, models.WSEvent{
		Type:       "player_left",
		GameID:     gameID,
		PlayerName: player.Name,
	})

	if game.Status == models.GameStatusFinished {
		s.hub.BroadcastGame(gameID, models.WSEvent{
			Type:   "game_ended",
			GameID: gameID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "left"})
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("id")
	token := r.Header.Get("Authorization")
	token = strings.TrimPrefix(token, "Bearer ")
	if token == "" {
		token = r.FormValue("token")
	}

	if err := r.ParseMultipartForm(256 << 20); err != nil {
		http.Error(w, "multipart inválido", http.StatusBadRequest)
		return
	}
	dateSaved := r.FormValue("date_saved")
	file, header, err := r.FormFile("vlog")
	if err != nil {
		http.Error(w, "campo vlog obrigatório", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "upload-*.vlog")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmp.Close()

	player, game, err := s.store.UploadTurn(r.Context(), gameID, token, dateSaved, tmpPath)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrNotYourTurn) || errors.Is(err, ErrGameNotActive) {
			status = http.StatusConflict
		} else if errors.Is(err, ErrInvalidToken) {
			status = http.StatusUnauthorized
		}
		http.Error(w, err.Error(), status)
		return
	}
	_ = header

	nextEmail, nextName, err := s.store.NextPlayerEmail(r.Context(), gameID, game.CurrentTurnIndex)
	if err == nil && nextEmail != "" {
		if err := s.mailer.SendTurnNotification(r.Context(), nextEmail, nextName, game.Name); err != nil {
			log.Printf("email error: %v", err)
		}
	}

	players, _ := s.store.playersByGame(r.Context(), gameID)
	tokens := make(map[string]bool)
	for _, p := range players {
		if p.TurnOrder == game.CurrentTurnIndex {
			tokens[p.ClientToken] = true
		}
	}
	s.hub.BroadcastTurnChanged(gameID, player.Name, dateSaved, tokens)

	writeJSON(w, http.StatusOK, map[string]string{"status": "uploaded"})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("id")
	token := r.URL.Query().Get("token")
	state, err := s.store.GameState(r.Context(), gameID, token)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrGameNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("id")
	rc, dateSaved, err := s.store.GetTurnReader(r.Context(), gameID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer rc.Close()

	filename := fmt.Sprintf("turn_%s.vlog", dateSaved)
	if dateSaved == "" {
		filename = "game.vlog"
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Content-Type", "application/zip")
	_, _ = io.Copy(w, rc)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("id")
	s.hub.HandleWS(w, r, gameID)
}

func (s *Server) handleJoinPage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	writeJSON(w, http.StatusOK, map[string]string{
		"invite_token": token,
		"message":      "Use o aplicativo Vassal vLog Sync para entrar nesta partida.",
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
