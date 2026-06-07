package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rafael/vassal-vlog-sync/pkg/models"
	_ "modernc.org/sqlite"
)

var (
	ErrGameNotFound       = errors.New("partida não encontrada")
	ErrNotYourTurn        = errors.New("não é seu turno")
	ErrGameNotActive      = errors.New("partida não está ativa")
	ErrInvalidToken       = errors.New("token inválido")
	ErrGameAlreadyStarted = errors.New("partida já iniciada")
)

type Store struct {
	db      *sql.DB
	driver  string
	blob    BlobStore
	dataDir string
	baseURL string
}

func (s *Store) q(sql string) string {
	if s.driver == "sqlite" {
		return sql
	}
	var b strings.Builder
	n := 1
	for _, r := range sql {
		if r == '?' {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			n++
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Store) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.q(query), args...)
}

func (s *Store) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.q(query), args...)
}

func (s *Store) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.q(query), args...)
}

func Open(driver, dsn, dataDir, baseURL string) (*Store, error) {
	if dataDir == "" {
		dataDir = "./data/vlogs"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	blob, err := NewBlobStore()
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}

	var db *sql.DB
	switch driver {
	case "postgres", "pgx":
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			return nil, err
		}
		db = stdlib.OpenDB(*cfg)
	default:
		driver = "sqlite"
		db, err = sql.Open("sqlite", dsn)
	}
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, driver: driver, blob: blob, dataDir: dataDir, baseURL: baseURL}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS games (
	id TEXT PRIMARY KEY,
	invite_token TEXT UNIQUE NOT NULL,
	name TEXT NOT NULL,
	vassal_module TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'lobby',
	current_turn_index INTEGER NOT NULL DEFAULT 0,
	host_player_id TEXT,
	created_at TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS players (
	id TEXT PRIMARY KEY,
	game_id TEXT NOT NULL REFERENCES games(id),
	name TEXT NOT NULL,
	email TEXT NOT NULL,
	client_token TEXT UNIQUE NOT NULL,
	turn_order INTEGER NOT NULL DEFAULT -1
);
CREATE TABLE IF NOT EXISTS turns (
	id TEXT PRIMARY KEY,
	game_id TEXT NOT NULL REFERENCES games(id),
	player_id TEXT NOT NULL REFERENCES players(id),
	vlog_path TEXT NOT NULL,
	date_saved TEXT NOT NULL,
	uploaded_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_players_game ON players(game_id);
CREATE INDEX IF NOT EXISTS idx_players_token ON players(client_token);
CREATE INDEX IF NOT EXISTS idx_games_invite ON games(invite_token);
`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) CreateGame(ctx context.Context, req models.CreateGameRequest) (*models.CreateGameResponse, error) {
	gameID := uuid.NewString()
	inviteToken := uuid.NewString()
	hostToken := uuid.NewString()
	hostID := uuid.NewString()
	now := time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		s.q(`INSERT INTO games (id, invite_token, name, vassal_module, status, current_turn_index, host_player_id, created_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`),
		gameID, inviteToken, req.Name, req.VassalModule, models.GameStatusLobby, hostID, now,
	)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx,
		s.q(`INSERT INTO players (id, game_id, name, email, client_token, turn_order) VALUES (?, ?, ?, ?, ?, 0)`),
		hostID, gameID, req.HostName, req.HostEmail, hostToken,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &models.CreateGameResponse{
		GameID:       gameID,
		InviteToken:  inviteToken,
		InviteURL:    fmt.Sprintf("%s/join/%s", strings.TrimRight(s.baseURL, "/"), inviteToken),
		VassalModule: req.VassalModule,
		GameName:     req.Name,
		ClientToken:  hostToken,
		TurnOrder:    0,
	}, nil
}

func (s *Store) JoinGame(ctx context.Context, req models.JoinRequest) (*models.JoinResponse, error) {
	game, err := s.gameByInviteToken(ctx, req.InviteToken)
	if err != nil {
		return nil, err
	}
	if game.Status != models.GameStatusLobby {
		return nil, ErrGameAlreadyStarted
	}

	playerID := uuid.NewString()
	clientToken := uuid.NewString()

	var count int
	err = s.queryRow(ctx, `SELECT COUNT(*) FROM players WHERE game_id = ?`, game.ID).Scan(&count)
	if err != nil {
		return nil, err
	}

	_, err = s.exec(ctx,
		`INSERT INTO players (id, game_id, name, email, client_token, turn_order) VALUES (?, ?, ?, ?, ?, ?)`,
		playerID, game.ID, req.Name, req.Email, clientToken, count,
	)
	if err != nil {
		return nil, err
	}

	return &models.JoinResponse{
		GameID:       game.ID,
		ClientToken:  clientToken,
		GameName:     game.Name,
		VassalModule: game.VassalModule,
		TurnOrder:    count,
	}, nil
}

func (s *Store) SetTurnOrder(ctx context.Context, gameID, hostToken string, playerIDs []string) error {
	host, game, err := s.playerByToken(ctx, hostToken)
	if err != nil {
		return err
	}
	if game.ID != gameID || game.HostPlayerID != host.ID {
		return ErrInvalidToken
	}
	if game.Status != models.GameStatusLobby {
		return ErrGameAlreadyStarted
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, pid := range playerIDs {
		res, err := tx.ExecContext(ctx,
			s.q(`UPDATE players SET turn_order = ? WHERE id = ? AND game_id = ?`), i, pid, gameID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("jogador %s não pertence à partida", pid)
		}
	}
	return tx.Commit()
}

func (s *Store) StartGame(ctx context.Context, gameID, hostToken string) error {
	host, game, err := s.playerByToken(ctx, hostToken)
	if err != nil {
		return err
	}
	if game.ID != gameID || game.HostPlayerID != host.ID {
		return ErrInvalidToken
	}
	if game.Status != models.GameStatusLobby {
		return ErrGameAlreadyStarted
	}

	var count int
	err = s.queryRow(ctx, `SELECT COUNT(*) FROM players WHERE game_id = ?`, gameID).Scan(&count)
	if err != nil {
		return err
	}
	if count < 2 {
		return errors.New("mínimo de 2 jogadores para iniciar")
	}

	_, err = s.exec(ctx,
		`UPDATE games SET status = ?, current_turn_index = 0 WHERE id = ?`,
		models.GameStatusActive, gameID)
	return err
}

func (s *Store) UploadTurn(ctx context.Context, gameID, clientToken, dateSaved, srcPath string) (*models.Player, *models.Game, error) {
	player, game, err := s.playerByToken(ctx, clientToken)
	if err != nil {
		return nil, nil, err
	}
	if game.ID != gameID {
		return nil, nil, ErrInvalidToken
	}
	if game.Status != models.GameStatusActive {
		return nil, nil, ErrGameNotActive
	}
	if player.TurnOrder != game.CurrentTurnIndex {
		return nil, nil, ErrNotYourTurn
	}

	turnID := uuid.NewString()
	blobKey := fmt.Sprintf("%s/%s.vlog", gameID, turnID)

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, nil, err
	}
	if err := s.blob.Put(ctx, blobKey, bytes.NewReader(data), int64(len(data))); err != nil {
		return nil, nil, fmt.Errorf("salvar vlog: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.exec(ctx,
		`INSERT INTO turns (id, game_id, player_id, vlog_path, date_saved, uploaded_at) VALUES (?, ?, ?, ?, ?, ?)`,
		turnID, gameID, player.ID, blobKey, dateSaved, now)
	if err != nil {
		return nil, nil, err
	}

	playerCount, err := s.playerCount(ctx, gameID)
	if err != nil {
		return nil, nil, err
	}
	nextIndex := (game.CurrentTurnIndex + 1) % playerCount
	_, err = s.exec(ctx,
		`UPDATE games SET current_turn_index = ? WHERE id = ?`, nextIndex, gameID)
	if err != nil {
		return nil, nil, err
	}

	game.CurrentTurnIndex = nextIndex
	return player, game, nil
}

func (s *Store) LeaveGame(ctx context.Context, gameID, clientToken string) (*models.Player, *models.Game, error) {
	player, game, err := s.playerByToken(ctx, clientToken)
	if err != nil {
		return nil, nil, err
	}
	if game.ID != gameID {
		return nil, nil, ErrInvalidToken
	}

	leavingOrder := player.TurnOrder

	if _, err := s.exec(ctx, `DELETE FROM players WHERE id = ?`, player.ID); err != nil {
		return nil, nil, err
	}

	count, err := s.playerCount(ctx, gameID)
	if err != nil {
		return nil, nil, err
	}

	if count < 2 {
		_, err = s.exec(ctx, `UPDATE games SET status = ? WHERE id = ?`, models.GameStatusFinished, gameID)
		if err != nil {
			return nil, nil, err
		}
		game.Status = models.GameStatusFinished
		return player, game, nil
	}

	_, err = s.exec(ctx,
		`UPDATE players SET turn_order = turn_order - 1 WHERE game_id = ? AND turn_order > ?`,
		gameID, leavingOrder)
	if err != nil {
		return nil, nil, err
	}

	if game.Status == models.GameStatusActive && game.CurrentTurnIndex > leavingOrder {
		game.CurrentTurnIndex--
		_, err = s.exec(ctx,
			`UPDATE games SET current_turn_index = ? WHERE id = ?`, game.CurrentTurnIndex, gameID)
		if err != nil {
			return nil, nil, err
		}
	}

	return player, game, nil
}

func (s *Store) GetTurnReader(ctx context.Context, gameID string) (io.ReadCloser, string, error) {
	key, dateSaved, err := s.LatestVlogPath(ctx, gameID)
	if err != nil {
		return nil, "", err
	}
	if key == "" {
		return nil, "", fmt.Errorf("nenhum .vlog disponível")
	}
	rc, err := s.blob.Get(ctx, key)
	if err != nil {
		return nil, "", fmt.Errorf("baixar vlog: %w", err)
	}
	return rc, dateSaved, nil
}

func (s *Store) LatestVlogPath(ctx context.Context, gameID string) (string, string, error) {
	var path, dateSaved string
	err := s.queryRow(ctx,
		`SELECT vlog_path, date_saved FROM turns WHERE game_id = ? ORDER BY uploaded_at DESC LIMIT 1`,
		gameID).Scan(&path, &dateSaved)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return path, dateSaved, err
}

func (s *Store) GameState(ctx context.Context, gameID, clientToken string) (*models.GameState, error) {
	game, err := s.gameByID(ctx, gameID)
	if err != nil {
		return nil, err
	}
	players, err := s.playersByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}

	state := &models.GameState{
		Game:    *game,
		Players: players,
	}

	if path, ds, err := s.LatestVlogPath(ctx, gameID); err == nil && path != "" {
		state.LastDateSaved = ds
		_ = path
	}

	var me *models.Player
	for i := range players {
		if players[i].ClientToken == clientToken {
			me = &players[i]
			state.YourTurnOrder = me.TurnOrder
			break
		}
	}
	if me != nil && game.Status == models.GameStatusActive {
		state.YourTurn = me.TurnOrder == game.CurrentTurnIndex
	}

	for i := range players {
		if players[i].TurnOrder == game.CurrentTurnIndex {
			p := players[i]
			p.ClientToken = ""
			state.CurrentPlayer = &p
			break
		}
	}

	for i := range state.Players {
		state.Players[i].ClientToken = ""
	}

	return state, nil
}

func (s *Store) NextPlayerEmail(ctx context.Context, gameID string, turnIndex int) (string, string, error) {
	var email, name string
	err := s.queryRow(ctx,
		`SELECT email, name FROM players WHERE game_id = ? AND turn_order = ?`, gameID, turnIndex).
		Scan(&email, &name)
	return email, name, err
}

func (s *Store) playerCount(ctx context.Context, gameID string) (int, error) {
	var count int
	err := s.queryRow(ctx, `SELECT COUNT(*) FROM players WHERE game_id = ?`, gameID).Scan(&count)
	return count, err
}

func (s *Store) gameByInviteToken(ctx context.Context, token string) (*models.Game, error) {
	row := s.queryRow(ctx,
		`SELECT id, invite_token, name, vassal_module, status, current_turn_index, host_player_id, created_at
		 FROM games WHERE invite_token = ?`, token)
	return scanGame(row)
}

func (s *Store) gameByID(ctx context.Context, id string) (*models.Game, error) {
	row := s.queryRow(ctx,
		`SELECT id, invite_token, name, vassal_module, status, current_turn_index, host_player_id, created_at
		 FROM games WHERE id = ?`, id)
	return scanGame(row)
}

func (s *Store) playerByToken(ctx context.Context, token string) (*models.Player, *models.Game, error) {
	var p models.Player
	err := s.queryRow(ctx,
		`SELECT id, game_id, name, email, client_token, turn_order FROM players WHERE client_token = ?`, token).
		Scan(&p.ID, &p.GameID, &p.Name, &p.Email, &p.ClientToken, &p.TurnOrder)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrInvalidToken
		}
		return nil, nil, err
	}
	game, err := s.gameByID(ctx, p.GameID)
	if err != nil {
		return nil, nil, err
	}
	return &p, game, nil
}

func (s *Store) playersByGame(ctx context.Context, gameID string) ([]models.Player, error) {
	rows, err := s.query(ctx,
		`SELECT id, game_id, name, email, client_token, turn_order FROM players WHERE game_id = ? ORDER BY turn_order`,
		gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []models.Player
	for rows.Next() {
		var p models.Player
		if err := rows.Scan(&p.ID, &p.GameID, &p.Name, &p.Email, &p.ClientToken, &p.TurnOrder); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

func scanGame(row *sql.Row) (*models.Game, error) {
	var g models.Game
	var status string
	err := row.Scan(&g.ID, &g.InviteToken, &g.Name, &g.VassalModule, &status,
		&g.CurrentTurnIndex, &g.HostPlayerID, &g.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrGameNotFound
		}
		return nil, err
	}
	g.Status = models.GameStatus(status)
	return &g, nil
}
