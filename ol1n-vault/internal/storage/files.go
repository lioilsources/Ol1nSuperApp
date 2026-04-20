package storage

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	vdb "github.com/lioilsources/ol1n-vault/internal/db"
)

// Cleaner periodically removes files whose download token has expired.
type Cleaner struct {
	Queries  *vdb.Queries
	FilesDir string
	Interval time.Duration
}

func (c *Cleaner) Start(ctx context.Context) {
	if c.Interval <= 0 {
		c.Interval = time.Hour
	}
	go func() {
		ticker := time.NewTicker(c.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.sweep(ctx)
			}
		}
	}()
}

func (c *Cleaner) sweep(ctx context.Context) {
	expired, err := c.Queries.DeleteExpiredTokens(ctx, time.Now().Unix())
	if err != nil {
		slog.Error("cleanup: delete expired tokens", "err", err)
		return
	}
	// For each expired token, remove the artifact directory if no other valid
	// token references that artifact. We check by trying to re-read any token
	// for the same artifact — if none remain with a live expiry, drop the dir.
	seen := map[string]bool{}
	for _, tok := range expired {
		if seen[tok.ArtifactID] {
			continue
		}
		seen[tok.ArtifactID] = true
		var count int
		err := c.Queries.DB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM download_tokens WHERE artifact_id = ? AND expires_at > ?`,
			tok.ArtifactID, time.Now().Unix(),
		).Scan(&count)
		if err != nil {
			slog.Error("cleanup: count live tokens", "err", err)
			continue
		}
		if count > 0 {
			continue
		}
		dir := filepath.Join(c.FilesDir, tok.ArtifactID)
		if err := os.RemoveAll(dir); err != nil {
			slog.Error("cleanup: remove dir", "dir", dir, "err", err)
		} else {
			slog.Info("cleanup: removed", "artifact_id", tok.ArtifactID)
		}
	}
}
