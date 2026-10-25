---
name: supabase-sync
description: Verify and change Call Analyzer's desktop vocabulary sync without contacting the hosted project.
---

# Supabase vocabulary sync

This skill is the desktop runner in `call_analyzer`. The hosted tables are defined in `../call-analyzer-lesson-db`. `../call-analyzer-active-recall` reads confirmed rows with a different Supabase secret and does not write `study_state` yet. Do not reuse this Mac's key for either of those.

Keep this path offline. Use `httptest.Server` and `storage.Open(ctx,
":memory:")`; never call the hosted Supabase project or put a secret, access
token, service key, or password in source, tests, logs, or docs.

## Behavior contract

- `Service.Initialize` starts one cross-platform vocabulary runner. The
  legacy `SyncEnabled` setting remains false and `internal/sync.Dispatcher`
  remains unused.
- The runner waits ten minutes from the newest undelivered eligible
  `vocabulary_entry` outbox row. It checks about every five seconds.
  Launch pulls confirmed vocabulary before that upload loop. `SyncNow`
  bypasses the quiet period and does not pull. Closing with rows that would be uploaded
  asks first; declining leaves those rows in the outbox. Tests inject the clock and
  quiet period.
- Confirmed and manually saved entries are eligible. A merged candidate is
  not uploaded; a confirmed merge target is republished. Candidate and
  rejected rows are acknowledged locally without HTTP. The runner reads the
  current SQLite row, collapses events by aggregate id, replaces senses,
  examples, and entry tags, and upserts tags by name.
- A missing local row or `vocabulary.entry.deleted` event deletes the hosted
  vocabulary row. `study.review.recorded` remains pending, does not block the
  vocabulary timer, and is never uploaded. Lessons, transcripts,
  observations, and study-state updates are outside this sync.
- A first confirmed/manual publish initializes `study_state` with
  `on_conflict=entry_id` and ignore-duplicates resolution.

## Secret lookup

The runner receives the app's secret store; it must not construct a default
store internally. `SUPABASE_SECRET_KEY` from the environment wins on every
OS. Only Darwin falls back to this Mac's Keychain service
`github.com/oeggy03.call_analyzer`, account `SUPABASE_SECRET_KEY`. Windows and
other non-Darwin builds have no Keychain and use only their own
`SUPABASE_SECRET_KEY`; never reuse or copy the Mac key.

The secret is sent only as PostgREST `apikey` and `Authorization: Bearer`
headers. It is never part of an `AppSnapshot`. Startup checks read one
vocabulary id and write then delete the tag `call-analyzer-access-probe`.
A replacement key is saved with the same secret store. The environment value
still wins, so a Keychain write does not override `SUPABASE_SECRET_KEY`.
