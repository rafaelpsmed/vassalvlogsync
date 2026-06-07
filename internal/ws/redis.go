package ws

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/rafael/vassal-vlog-sync/pkg/models"
	"github.com/redis/go-redis/v9"
)

const wsPrefix = "ws:broadcast:"

func (h *Hub) initRedis() *redis.Client {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		return nil
	}
	password := os.Getenv("REDIS_PASSWORD")
	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
	})
}

func (h *Hub) redisPublish(gameID string, event models.WSEvent) {
	if h.redisClient == nil {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	if err := h.redisClient.Publish(context.Background(), wsPrefix+gameID, data).Err(); err != nil {
		log.Printf("redis publish error: %v", err)
	}
}

func (h *Hub) redisSubscribe(ctx context.Context) {
	if h.redisClient == nil {
		return
	}
	pubsub := h.redisClient.PSubscribe(ctx, wsPrefix+"*")
	ch := pubsub.Channel()
	log.Println("ws: redis pub/sub ativo")

	go func() {
		<-ctx.Done()
		pubsub.Close()
	}()

	for msg := range ch {
		var event models.WSEvent
		if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
			continue
		}
		h.broadcastLocal(event)
	}
}
