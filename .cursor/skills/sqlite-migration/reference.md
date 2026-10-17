# SQLite migration reference

| Piece | Path |
| --- | --- |
| Embedded migrations | `internal/storage/migrations/*.sql` |
| Apply order and `schema_migrations` | `internal/storage/store.go` |
| Lesson columns | `internal/storage/lessons.go` |
| Vocabulary, senses, examples, tags | `internal/storage/vocabulary.go` |
| Outbox and usage | `internal/storage/outbox.go` |
| Study state | `internal/storage/study.go` |
| Row structs | `internal/domain/domain.go` |
| Tests | `internal/storage/storage_test.go` |

Pragmas set at open: foreign keys, WAL, `busy_timeout` 5000, `synchronous` NORMAL. The file mode for a path database is `0600`.

Runtime path: `os.UserConfigDir()/call_analyzer/call_analyzer.db`, created in `NewApp`. Tests must not open that file.
