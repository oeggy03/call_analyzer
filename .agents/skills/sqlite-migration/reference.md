# SQLite reference

| Concern | Source |
| --- | --- |
| Embedded migrations | `internal/storage/migrations/*.sql` |
| Apply order, transactions, and migration records | `internal/storage/store.go` |
| Lesson and transcript repositories | `internal/storage/lessons.go` |
| Vocabulary and cloud-pull repositories | `internal/storage/vocabulary.go`, `internal/storage/vocabulary_cloud.go` |
| Outbox, observations, settings, and usage repositories | `internal/storage/outbox.go` |
| Study state | `internal/storage/study.go` |
| Domain row shapes | `internal/domain/domain.go` |
| Storage tests | `internal/storage/storage_test.go` |

`storage.Open` enables foreign keys, WAL, a 5000 ms busy timeout, and `synchronous=NORMAL`. A path-backed database uses restrictive file permissions. The runtime database is under the user's config directory; tests must use an in-memory database or a task-local temporary path.
