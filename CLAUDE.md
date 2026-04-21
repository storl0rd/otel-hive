# otel-hive

OpAMP-based OTel Collector fleet management server. Forked from Lawrence OSS (Apache 2.0), stripped to a management plane, extended with Git-based config sync, built-in auth, and multi-environment support.

## Project context
- Companion project to [otel.guru](https://otel.guru) — lives at `storl0rd/otel-hive` on GitHub
- Goal: make OTel Collector fleet management accessible to junior operators
- Standalone Docker binary; no external database required (SQLite)
- See full system design: `/Users/aether/.claude/plans/take-the-content-from-compressed-fern.md`

## Stack
- **Backend:** Go 1.24+, Gin, SQLite (`mattn/go-sqlite3`), `open-telemetry/opamp-go` SDK
- **Frontend:** React 19, TypeScript, Vite, Tailwind CSS, Radix UI, Monaco Editor
- **Build:** CGO enabled (SQLite); single binary embeds React build via `embed`
- **Deploy:** Docker, docker-compose; future Helm chart

## Dev commands
```bash
# Backend
export PATH="/opt/homebrew/bin:$PATH"
go run ./cmd/all-in-one --config ./otel-hive.yaml

# Frontend
cd ui && npm install && npm run dev   # Vite dev server on :5173

# Build check
go build ./...
cd ui && npm run build
```

## Ports
- `8080` — HTTP API + embedded React UI
- `4320` — OpAMP WebSocket (collectors connect here)

## Module name
`github.com/storl0rd/otel-hive` — all Go imports use this path

## Key packages
- `internal/opamp/` — OpAMP WebSocket server (keep, do not touch)
- `internal/storage/applicationstore/` — SQLite app store (keep)
- `internal/services/agent_service*.go` — AgentService (keep)
- `internal/api/` — Gin HTTP server + handlers
- `internal/auth/` — JWT + bcrypt + API keys (Phase 2, to be created)
- `internal/gitsync/` — Git sync service (Phase 3, to be created)

## Implementation phases
- [x] Phase 1 — Fork & strip (done, pushed)
- [ ] Phase 2 — Built-in auth (JWT, bcrypt, API keys, login UI)
- [ ] Phase 3 — Git sync service (GitHub/GitLab/Gitea/raw HTTP)
- [ ] Phase 4 — Multi-environment support
- [ ] Phase 5 — UX polish, templates, setup wizard

## Task tracking
See `tasks/todo.md` for detailed checklist.
See `tasks/lessons.md` for corrections log.

## Coding standards
- Simplicity first — minimal impact per change
- No comments unless the WHY is non-obvious
- No premature abstractions — build for what's needed now
- Senior developer bar: find root causes, no temp fixes
- Run `go build ./...` + `cd ui && npm run build` before marking anything done
- Never push to main directly — feature branches + PRs

## Git workflow
- Branch naming: `feature/<desc>`, `fix/<desc>`, `chore/<desc>`
- Never force push, never skip hooks
- SSH key: `~/.ssh/github_ed25519`
