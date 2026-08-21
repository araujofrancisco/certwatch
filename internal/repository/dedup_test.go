package repository

import (
	"context"
	"testing"
	"time"
)

func TestNotificationDedupRepository(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	defer db.Close()
	repo := NewNotificationDedupRepository(db)

	key := "42:7"

	seen, err := repo.Seen(ctx, key)
	if err != nil {
		t.Fatalf("Seen: %v", err)
	}
	if seen {
		t.Fatal("expected key to not be seen initially")
	}

	if err := repo.Mark(ctx, key, time.Now()); err != nil {
		t.Fatalf("Mark: %v", err)
	}

	seen, err = repo.Seen(ctx, key)
	if err != nil {
		t.Fatalf("Seen after Mark: %v", err)
	}
	if !seen {
		t.Fatal("expected key to be seen after Mark")
	}

	// Re-Mark refreshes the timestamp (idempotent upsert).
	newer := time.Now().Add(2 * time.Hour)
	if err := repo.Mark(ctx, key, newer); err != nil {
		t.Fatalf("re-Mark: %v", err)
	}

	// Cleanup removes only entries older than the cutoff.
	n, err := repo.Cleanup(ctx, newer.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 deletions before expiry, got %d", n)
	}

	n, err = repo.Cleanup(ctx, newer.Add(time.Hour))
	if err != nil {
		t.Fatalf("Cleanup expired: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 deletion after expiry, got %d", n)
	}

	seen, err = repo.Seen(ctx, key)
	if err != nil {
		t.Fatalf("Seen after cleanup: %v", err)
	}
	if seen {
		t.Fatal("expected key to be gone after cleanup")
	}
}
