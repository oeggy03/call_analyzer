---
name: supabase-sync
description: Cursor pointer to the canonical Call Analyzer vocabulary sync skill.
---

Read the canonical skill at
`.agents/skills/supabase-sync/SKILL.md` before changing desktop vocabulary
sync. It defines the launch pull, the ten-minute quiet period, eligible and
excluded outbox events, current-row uploads, and per-machine Supabase secret
lookup. `SyncNow` uploads only. Startup checks that the secret can read and write.
