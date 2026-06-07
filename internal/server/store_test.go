package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rafael/vassal-vlog-sync/internal/server"
	"github.com/rafael/vassal-vlog-sync/internal/ws"
	"github.com/rafael/vassal-vlog-sync/pkg/models"
)

func TestGameFlow(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	dataDir := filepath.Join(dir, "vlogs")

	store, err := server.Open("sqlite", dbPath, dataDir, "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	hub := ws.NewHub()
	srv := server.NewServer(store, hub, server.NewMailerFromEnv())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx := context.Background()

	createBody, _ := json.Marshal(models.CreateGameRequest{
		Name:         "Test Game",
		VassalModule: "Test Module",
		HostName:     "Alice",
		HostEmail:    "alice@test.com",
	})
	resp, err := http.Post(ts.URL+"/games", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create: %d %s", resp.StatusCode, b)
	}
	var created models.CreateGameResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	joinBody, _ := json.Marshal(models.JoinRequest{
		InviteToken: created.InviteToken,
		Name:        "Bob",
		Email:       "bob@test.com",
	})
	resp, err = http.Post(ts.URL+"/join", "application/json", bytes.NewReader(joinBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("join: %d %s", resp.StatusCode, b)
	}
	var joined models.JoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&joined); err != nil {
		t.Fatal(err)
	}

	startReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/games/"+created.GameID+"/start", nil)
	startReq.Header.Set("Authorization", "Bearer "+created.ClientToken)
	resp, err = http.DefaultClient.Do(startReq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start: %d", resp.StatusCode)
	}

	vlogPath := filepath.Join(dir, "sample.vlog")
	if err := os.WriteFile(vlogPath, []byte("not-a-real-vlog"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("vlog", "sample.vlog")
	_, _ = part.Write([]byte("fake vlog content"))
	_ = writer.WriteField("date_saved", "1234567890")
	writer.Close()

	uploadReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/games/"+created.GameID+"/upload", body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadReq.Header.Set("Authorization", "Bearer "+created.ClientToken)
	resp, err = http.DefaultClient.Do(uploadReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload: %d %s", resp.StatusCode, b)
	}

	state, err := store.GameState(ctx, created.GameID, joined.ClientToken)
	if err != nil {
		t.Fatal(err)
	}
	if state.YourTurn != true {
		t.Fatalf("expected Bob's turn, got your_turn=%v", state.YourTurn)
	}

	dlResp, err := http.Get(ts.URL + "/games/" + created.GameID + "/download")
	if err != nil {
		t.Fatal(err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("download: %d", dlResp.StatusCode)
	}
}
