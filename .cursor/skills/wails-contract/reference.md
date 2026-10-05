# Wails contract reference

| Piece | Path |
| --- | --- |
| Methods | `app.go` |
| Input and snapshot structs | `app_contract.go` |
| Domain-to-snapshot mapping | `snapshot.go` |
| UI method names and both clients | `frontend/src/lib/api.ts` |
| UI types | `frontend/src/lib/types.ts` |
| Screen behavior | `frontend/src/App.tsx` |
| Contract test | `contract_test.go` |
| Events | `call_analyzer:` prefix in `app.go`; names in `BACKEND_EVENTS` |

`GetAppSnapshot` and `Refresh` return the current snapshot. Mutating methods return the snapshot after the write, except `GenerateManualVocabulary`.

Settings key fields are write-only. See `SaveSettings` in `app.go`.
