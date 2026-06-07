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
