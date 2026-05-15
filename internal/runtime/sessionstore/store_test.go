package sessionstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreInitializesAndRoundTripsRecord(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 15, 12, 0, 0, 123, time.UTC)
	record := Record{
		IssueID:         "issue-1",
		IssueIdentifier: "ZEE-1",
		ThreadID:        "thread-1",
		WorkspacePath:   "/repo/.worktrees/ZEE-1",
		WorkerHost:      "devbox-1",
		LastState:       "Human Review",
		LastSessionID:   "thread-1-turn-1",
		LastTurnID:      "turn-1",
		UpdatedAt:       now,
	}

	if err := store.Upsert(ctx, record); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	got, err := store.Get(ctx, "issue-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got != record {
		t.Fatalf("record = %#v, want %#v", got, record)
	}
}

func TestStoreUpsertOverwritesRecord(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.Upsert(ctx, Record{IssueID: "issue-1", IssueIdentifier: "ZEE-1", ThreadID: "thread-1", WorkspacePath: "/one", LastState: "Human Review"}); err != nil {
		t.Fatalf("initial Upsert returned error: %v", err)
	}
	if err := store.Upsert(ctx, Record{IssueID: "issue-1", IssueIdentifier: "ZEE-1", ThreadID: "thread-2", WorkspacePath: "/two", LastState: "Rework"}); err != nil {
		t.Fatalf("second Upsert returned error: %v", err)
	}
	got, err := store.Get(ctx, "issue-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.ThreadID != "thread-2" || got.WorkspacePath != "/two" || got.LastState != "Rework" {
		t.Fatalf("record = %#v, want overwritten fields", got)
	}
}

func TestStoreGetMissAndDelete(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if _, err := store.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing error = %v, want ErrNotFound", err)
	}
	if err := store.Upsert(ctx, Record{IssueID: "issue-1", IssueIdentifier: "ZEE-1", ThreadID: "thread-1", WorkspacePath: "/one", LastState: "Human Review"}); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	if err := store.Delete(ctx, "issue-1"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := store.Get(ctx, "issue-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get deleted error = %v, want ErrNotFound", err)
	}
}

func TestStoreClose(t *testing.T) {
	store := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "state", "sessions.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}
