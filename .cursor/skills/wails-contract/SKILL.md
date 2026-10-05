---
name: wails-contract
description: >-
  Updates the Call Analyzer Wails snapshot contract across Go and React.
  Use when adding or changing an App method, AppSnapshot field, candidate
  edit, settings patch, BACKEND_METHODS, or the demo API client.
---

# Wails contract

## When

Use this when a desktop feature changes what the UI can call or what `AppSnapshot` contains. Do not use it for a CSS-only change.

## Inputs

- The method or snapshot field to add or change.
- Whether the demo client can implement it without a network call. It must.

## Procedure

1. Read `docs/architecture.md` (the UI and Wails rows) and `frontend/README.md`.
2. Change the exported method on `App` in `app.go`. Put request structs in `app_contract.go`. Keep JSON camelCase.
3. Map `pinyin` to `domain.CandidateEdit.Reading` if the edit patch grows. Follow `App.EditCandidate`.
4. Return `(AppSnapshot, error)` unless the call is a non-persistent draft like `GenerateManualVocabulary`.
5. Add the Go method name to `BACKEND_METHODS` in `frontend/src/lib/api.ts`. Update `AppApi`, `RuntimeApi`, and `DemoApi`.
6. Update `frontend/src/lib/types.ts` when the snapshot or input shape changes. Keep UI code on `api.ts`.
7. Do not put the API key, audio bytes, or filesystem paths into the snapshot.

## Verification

- `go test -count=1 -run TestWailsMethodsMatchFrontendContract .`
- `cd frontend && npm run typecheck && npm test -- --run`
- For a behavior change, also run the Go package you edited.

Success: the contract test passes, and both the runtime and demo clients accept the same arguments.

## Failure cases

- The contract test cannot see `BACKEND_METHODS`: the export block in `api.ts` was renamed or split.
- Typecheck passes but the desktop app throws: `DemoApi` was updated and `RuntimeApi` was not, or the Go method is unexported.
- `GenerateManualVocabulary` was changed to return `AppSnapshot`: callers expect `ManualVocabularyDraft`.

## References

- [reference.md](reference.md)
