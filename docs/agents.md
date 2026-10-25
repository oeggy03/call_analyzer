# Agent roles

This page describes the repository's Codex roles and how to split work across its boundaries. The source code remains the authority for app behavior and API shapes. Read the relevant package and trace callers before proposing a change; use docs for workflow and architecture context.

## Model assignments

The repository orchestrator and planner use `gpt-6.1-sol` with high reasoning. Every implementation role uses `gpt-6-luna` with xhigh reasoning, including frontend, backend, native capture, tooling, and docs. These assignments live in `.codex/config.toml` and `.codex/agents/*.toml`. If the active session does not load the configured model or it is unavailable, report the mismatch rather than silently changing models. The role configuration does not grant extra filesystem, approval, or external-action permissions.

## Activation

Project settings apply when the trusted repository is loaded in a new session; they do not change a running session. Explicit CLI or UI model selection can override repository defaults, so check the effective model and report a mismatch instead of silently falling back. Standalone worker roles live in `.codex/agents/*.toml`, while skills in `.agents/skills` are discovered automatically; name the desired worker role in implementation prompts.

For the supported role configuration and skill discovery formats, see [Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents) and [building Codex skills](https://learn.chatgpt.com/docs/build-skills).

## Role boundaries

| Role | Owns | Read first |
| --- | --- | --- |
| `planner` | Task framing, package ownership, cross-boundary review, integration, and final validation. It does not implement repository changes; it delegates those to the matching Luna implementation role. | Root `AGENTS.md`, relevant source and docs |
| `frontend` | React screens, styles, frontend types and API clients. When changing the Wails boundary, coordinate its Go side through the contract files. | `frontend/README.md`, `docs/architecture.md`, `wails-contract` |
| `backend` | Go app methods, service, domain, OpenRouter, vocabulary, SQLite repositories, and the desktop vocabulary sync runner. Coordinate any UI-facing shape through the Wails contract. Shared Postgres columns start in `../call-analyzer-lesson-db`, not in this repo. | `docs/architecture.md`, relevant package tests, `wails-contract`, `sqlite-migration`, or `supabase-sync` |
| `native-capture` | Swift capture helper and Go macOS adapter as one process boundary. Keep platform stubs compiling. | Capture invariants in root `AGENTS.md`, `internal/capture/macos/README.md` |
| `tooling-docs` | Setup, build, verification scripts, CI, and task-relevant documentation. Describe behavior after checking the implementation. | `docs/development.md`, `docs/operations.md`, relevant source |

These four implementation roles match the repository's distinct ownership areas. Do not add separate reviewer or verifier roles by default: the planner integrates results, and `verify-call-analyzer` covers the repeatable check workflow. Use no more parallel workers than the task can be integrated safely; the repository setting currently allows three concurrent subagent threads.

## Delegation and integration

The repository instructions authorize the orchestrator to delegate implementation work that is within the user's requested scope; the user does not need to repeat that authorization. Give each worker a bounded, non-overlapping file/domain scope, the expected result, and behavior constraints. Keep one owner for files shared across boundaries. The planner resolves overlaps, compares proposed changes with current code, reviews generated artifacts as outputs, and runs final relevant checks. Skills are workflow guidance; they do not override user scope or current code.

## Evidence for documentation

Before changing behavior documentation, identify the code path that implements the claim. Distinguish validation of a setting from runtime behavior, and distinguish UI display state from service enforcement. For example, `hardBudgetUsd` remains a settings/display field with legacy validation, while `Budget.Check` and the current `HardExceeded` status do not stop cloud calls. Also check request-specific privacy flags: transcription and standard chat request ZDR and denied data collection, while vision photo-recognition omits those flags. Keep distinctions explicit and re-check code whenever a later change may alter them.
