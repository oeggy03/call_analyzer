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
	"regexp"
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
	localFile := !inMemory && !strings.HasPrefix(path, "file:")
	if inMemory {
		dsn = "file:call-analyzer-" + uuid.NewString() + "?mode=memory&cache=shared"
	} else if localFile {
		directory := filepath.Dir(path)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("storage: create database directory: %w", err)
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return nil, fmt.Errorf("storage: protect database directory: %w", err)
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
	if localFile {
		if err := os.Chmod(path, 0o600); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("storage: protect database file: %w", err)
		}
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
		satisfied, err := migrationAddsOnlyExistingColumns(ctx, tx, string(script))
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("storage: inspect migration %s: %w", entry.Name(), err)
		}
		if !satisfied {
			if _, err := tx.ExecContext(ctx, string(script)); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("storage: apply migration %s: %w", entry.Name(), err)
			}
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

var addColumnStatement = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+([A-Za-z_][A-Za-z0-9_]*)\s+ADD\s+COLUMN\s+([A-Za-z_][A-Za-z0-9_]*)\b`)

// migrationAddsOnlyExistingColumns reports whether a migration consists only
// of ADD COLUMN statements for columns that are already present. Fresh
// databases create those columns in 001_init.sql; databases created before a
// column was added to that file need the later ALTER.
func migrationAddsOnlyExistingColumns(ctx context.Context, tx *sql.Tx, script string) (bool, error) {
	statements := splitSQLStatements(script)
	if len(statements) == 0 {
		return false, nil
	}
	for _, statement := range statements {
		matches := addColumnStatement.FindStringSubmatch(statement)
		if matches == nil {
			return false, nil
		}
		var present int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(1) FROM pragma_table_info(?)
			WHERE name = ?`, matches[1], matches[2]).Scan(&present); err != nil {
			return false, err
		}
		if present == 0 {
			return false, nil
		}
	}
	return true, nil
}

func splitSQLStatements(script string) []string {
	parts := strings.Split(script, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		statement := strings.TrimSpace(part)
		if statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
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
