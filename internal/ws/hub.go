package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rafael/vassal-vlog-sync/pkg/models"
	"github.com/redis/go-redis/v9"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type client struct {
	gameID      string
	clientToken string
	conn        *websocket.Conn
}

type Hub struct {
	mu          sync.RWMutex
	clients     map[*client]struct{}
	redisClient *redis.Client
	redisCancel context.CancelFunc
}

func NewHub() *Hub {
	h := &Hub{clients: make(map[*client]struct{})}
	h.redisClient = h.initRedis()
	if h.redisClient != nil {
		ctx, cancel := context.WithCancel(context.Background())
		h.redisCancel = cancel
		go h.redisSubscribe(ctx)
	}
	return h
}

func (h *Hub) Close() {
	if h.redisCancel != nil {
		h.redisCancel()
	}
	if h.redisClient != nil {
		h.redisClient.Close()
	}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request, gameID string) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token obrigatório", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	c := &client{gameID: gameID, clientToken: token, conn: conn}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *Hub) BroadcastGame(gameID string, event models.WSEvent) {
	h.redisPublish(gameID, event)
	h.broadcastLocal(event)
}

func (h *Hub) BroadcastTurnChanged(gameID string, currentPlayerName, dateSaved string, nextPlayerTokens map[string]bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		if c.gameID != gameID {
			continue
		}
		yourTurn := nextPlayerTokens[c.clientToken]
		event := models.WSEvent{
			Type:          "turn_changed",
			GameID:        gameID,
			YourTurn:      yourTurn,
			CurrentPlayer: currentPlayerName,
			DateSaved:     dateSaved,
		}
		data, _ := json.Marshal(event)
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("ws write error: %v", err)
		}
	}

	h.redisPublish(gameID, models.WSEvent{
		Type:          "turn_changed",
		GameID:        gameID,
		CurrentPlayer: currentPlayerName,
		DateSaved:     dateSaved,
	})
}

func (h *Hub) BroadcastPlayerJoined(gameID, playerName string) {
	h.BroadcastGame(gameID, models.WSEvent{
		Type:       "player_joined",
		GameID:     gameID,
		PlayerName: playerName,
	})
}

func (h *Hub) broadcastLocal(event models.WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.gameID != event.GameID {
			continue
		}
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("ws write error: %v", err)
		}
	}
}

func (h *Hub) makeRedisClient(addr, password string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
	})
}
