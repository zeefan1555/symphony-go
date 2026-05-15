package sessionstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("session record not found")

type Store struct {
	db *sql.DB
}

type Record struct {
	IssueID         string
	IssueIdentifier string
	ThreadID        string
	WorkspacePath   string
	WorkerHost      string
	LastState       string
	LastSessionID   string
	LastTurnID      string
	UpdatedAt       time.Time
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("session store path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func DefaultPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".symphony", "state", "sessions.db")
}

func (s *Store) init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS issue_sessions (
	issue_id TEXT PRIMARY KEY,
	issue_identifier TEXT NOT NULL,
	thread_id TEXT NOT NULL,
	workspace_path TEXT NOT NULL,
	worker_host TEXT NOT NULL DEFAULT '',
	last_state TEXT NOT NULL,
	last_session_id TEXT NOT NULL DEFAULT '',
	last_turn_id TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);
`)
	return err
}

func (s *Store) Get(ctx context.Context, issueID string) (Record, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT issue_id, issue_identifier, thread_id, workspace_path, worker_host, last_state, last_session_id, last_turn_id, updated_at
FROM issue_sessions
WHERE issue_id = ?`, issueID)

	var record Record
	var updatedAt string
	if err := row.Scan(
		&record.IssueID,
		&record.IssueIdentifier,
		&record.ThreadID,
		&record.WorkspacePath,
		&record.WorkerHost,
		&record.LastState,
		&record.LastSessionID,
		&record.LastTurnID,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Record{}, ErrNotFound
		}
		return Record{}, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Record{}, err
	}
	record.UpdatedAt = parsed
	return record, nil
}

func (s *Store) Upsert(ctx context.Context, record Record) error {
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO issue_sessions (
	issue_id, issue_identifier, thread_id, workspace_path, worker_host, last_state, last_session_id, last_turn_id, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(issue_id) DO UPDATE SET
	issue_identifier = excluded.issue_identifier,
	thread_id = excluded.thread_id,
	workspace_path = excluded.workspace_path,
	worker_host = excluded.worker_host,
	last_state = excluded.last_state,
	last_session_id = excluded.last_session_id,
	last_turn_id = excluded.last_turn_id,
	updated_at = excluded.updated_at`,
		record.IssueID,
		record.IssueIdentifier,
		record.ThreadID,
		record.WorkspacePath,
		record.WorkerHost,
		record.LastState,
		record.LastSessionID,
		record.LastTurnID,
		record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) Delete(ctx context.Context, issueID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM issue_sessions WHERE issue_id = ?`, issueID)
	return err
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
