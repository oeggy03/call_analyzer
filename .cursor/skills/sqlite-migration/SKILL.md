---
name: sqlite-migration
description: >-
  Adds a Call Analyzer SQLite migration and repository update. Use when
  changing tables, columns, indexes, or embedded SQL under
  internal/storage/migrations.
---

# SQLite migration

## When

Use this for a schema change. Do not use it to tweak a query that the existing tables already support.

## Inputs

- The schema change.
- Which repository methods read or write the affected rows.

## Prerequisites

- Go 1.25+.
- No running app is required. Tests use `:memory:`.

## Procedure

1. Read `docs/architecture.md` (Database) and `internal/storage/store.go` (`migrate`).
2. Add the next numbered file, `internal/storage/migrations/00N_description.sql`. Do not edit `001_init.sql` or `002_lesson_metadata.sql` once they may have been applied. `schema_migrations` records the filename.
3. Keep the SQL valid as one `Exec` of the whole file. `002_lesson_metadata.sql` is the existing multi-statement example.
4. Update the repository in `internal/storage` and the domain struct in `internal/domain/domain.go` if the row shape changed.
5. If the UI shows the new field, follow the `wails-contract` skill. A column that only the service reads does not need a snapshot field.
6. Extend `internal/storage/storage_test.go` for the new read or write. Use `storage.Open(ctx, ":memory:")`.

## Verification

```sh
gofmt -l internal/storage internal/domain
go test ./internal/storage ./internal/domain
```

Success: the new migration applies on an empty database, and a second `Open` does not re-run it (`schema_migrations`).

## Failure cases

- Editing `001` or `002` instead of adding `003`: existing local databases will not pick up the change, because those names are already recorded.
- A migration that fails partway: `migrate` runs each file in a transaction. Fix the SQL; do not hand-edit a user's database.
- Putting the OpenRouter key in a new column: store it through `SecretStore` instead. See `docs/decisions.md`.

## References

- [reference.md](reference.md)
