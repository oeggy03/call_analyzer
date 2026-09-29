package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Store struct {
	db   *sql.DB
	path string
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("storage: database path is required")
	}

	dsn := path
	inMemory := path == ":memory:"
	if inMemory {
		dsn = "file:call-analyzer-" + uuid.NewString() + "?mode=memory&cache=shared"
	} else if !strings.HasPrefix(path, "file:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("storage: create database directory: %w", err)
		}
	}
	dsn = withSQLitePragmas(dsn)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open sqlite: %w", err)
	}
	if inMemory {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	} else {
		db.SetMaxOpenConns(8)
		db.SetMaxIdleConns(8)
	}

	store := &Store{db: db, path: path}
	if err := store.configure(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) configure(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	}
	for _, pragma := range pragmas {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("storage: configure sqlite (%s): %w", pragma, err)
		}
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("storage: ping sqlite: %w", err)
	}
	return nil
}

func withSQLitePragmas(dsn string) string {
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator +
		"_pragma=foreign_keys(1)&" +
		"_pragma=busy_timeout(5000)&" +
		"_pragma=journal_mode(WAL)&" +
		"_pragma=synchronous(NORMAL)"
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY NOT NULL,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		return fmt.Errorf("storage: create migration table: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("storage: list migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var applied int
		err := s.db.QueryRowContext(ctx,
			"SELECT COUNT(1) FROM schema_migrations WHERE name = ?",
			entry.Name(),
		).Scan(&applied)
		if err != nil {
			return fmt.Errorf("storage: check migration %s: %w", entry.Name(), err)
		}
		if applied > 0 {
			continue
		}

		script, err := migrationFiles.ReadFile(filepath.Join("migrations", entry.Name()))
		if err != nil {
			return fmt.Errorf("storage: read migration %s: %w", entry.Name(), err)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("storage: begin migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.ExecContext(ctx, string(script)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("storage: apply migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations(name, applied_at) VALUES (?, ?)",
			entry.Name(), time.Now().UnixMilli(),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("storage: record migration %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("storage: commit migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func newID() string {
	return uuid.NewString()
}

func timeValue(value time.Time) int64 {
	return value.UnixMilli()
}

func timeFromValue(value int64) time.Time {
	return time.UnixMilli(value).UTC()
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return timeValue(*value)
}

func scanNullableTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	t := timeFromValue(value.Int64)
	return &t
}
