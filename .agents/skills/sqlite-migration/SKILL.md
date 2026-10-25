---
name: sqlite-migration
description: >-
  Add or update a Call Analyzer SQLite schema migration and its repository
  consumers. Use for tables, columns, indexes, or embedded SQL changes; skip
  for query-only changes supported by the current schema.
---

# SQLite migration

Use this for a SQLite schema change. Read the Database section of `docs/architecture.md` and inspect `internal/storage/store.go`, the affected repository, and its callers.

A column that confirmed vocabulary must share with the recall service is a Postgres change first. Add that migration in `../call-analyzer-lesson-db` and apply it before changing this app's sync mapping. Local-only tables (`outbox`, `settings`, `request_usage`) stay in this repository.

## Workflow

1. Inspect `internal/storage/migrations` and derive the next numeric prefix from the latest migration already present. At the time of writing, files `001` through `004` exist, so the next new filename starts with `005_`. Do not assume that remains current.
2. Add a new migration file; never edit or remove a migration that may already have been applied. The runner sorts filenames, records each filename in `schema_migrations`, and applies each file inside a transaction.
3. Update repository methods and domain/service structs for the new shape. If the UI consumes the field, follow `wails-contract`; service-only fields do not need a Wails snapshot field.
4. Add a storage test for the affected read/write behavior. Use `storage.Open(ctx, ":memory:")` for ordinary schema tests; use a temporary directory only when persistence across opens is the behavior under test. Never use the real user database.
5. Keep a migration valid for the runner's single `Exec` call of its full SQL content. Inspect existing migration conventions before adding special SQL.

The runner has a narrow compatibility path for `ALTER TABLE ... ADD COLUMN` when the column already exists; do not rely on that as a substitute for a new migration or as a reason to rewrite historical SQL.

## Verification

Run the SQL migration row in the focused check matrix in `docs/development.md`. Confirm that the new migration applies on an empty in-memory database and that reopening the same test database does not record or apply it twice.

See [reference.md](reference.md) for schema, repository, and test locations.
