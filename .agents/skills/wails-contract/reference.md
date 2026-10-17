# Wails contract reference

| Concern | Source |
| --- | --- |
| Exported methods and app behavior | `app.go` |
| Wails inputs and output structs | `app_contract.go` |
| Domain-to-snapshot mapping | `snapshot.go` |
| UI method names, runtime and demo clients | `frontend/src/lib/api.ts` |
| Frontend data types | `frontend/src/lib/types.ts` |
| Screen behavior | `frontend/src/App.tsx` |
| Contract test | `contract_test.go` |
| Event names | `call_analyzer:` names in `app.go` and `BACKEND_EVENTS` in `api.ts` |

The exported methods and frontend types are authoritative for return shapes. Generated binding files are outputs, not a second source of truth.
