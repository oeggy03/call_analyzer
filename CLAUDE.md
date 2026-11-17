Follow `AGENTS.md` and `docs/agents.md`.

This workspace is the desktop app (`call_analyzer`, including the React Wails UI, Go backend, and the `native/macos-capture` and `native/macos-lookup` Swift helpers). The shared Postgres migrations are in `../call-analyzer-lesson-db`. The live 9:00 Telegram recall job is in `../call-analyzer-active-recall`. Do not edit those repositories unless that workspace is open.

Use the canonical project skills in `.agents/skills` (`verify-call-analyzer`, `wails-contract`, `sqlite-migration`, `supabase-sync`). Keep all verification offline (`./scripts/verify.sh`; never pass `-execute` to `cmd/evaluate` or contact the hosted Supabase project unless asked). Never commit an OpenRouter, Supabase, or Telegram secret, never log or expose `Command-Option-5` selection text in `AppSnapshot`, and never copy this Mac's `SUPABASE_SECRET_KEY` to Windows or into the recall service.
