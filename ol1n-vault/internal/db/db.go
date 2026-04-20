package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed migrations/001_init.sql
var migration001 string

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("db: mkdir: %w", err)
	}
	d, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	if err := d.Ping(); err != nil {
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	if _, err := d.Exec(migration001); err != nil {
		return nil, fmt.Errorf("db: migrate 001: %w", err)
	}
	return d, nil
}
