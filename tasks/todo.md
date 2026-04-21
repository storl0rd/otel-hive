# otel-hive — Task Tracker

## Phase 1 — Fork & Strip ✅ (2026-04-20)
- [x] Fork `getlawrence/lawrence-oss` → `storl0rd/otel-hive`
- [x] Rename Go module to `github.com/storl0rd/otel-hive`
- [x] Remove `internal/otlp/`, `internal/storage/telemetrystore/`, `internal/query/`, `internal/worker/`
- [x] Remove telemetry/topology/lawrence-ql API handlers and services
- [x] Remove `@clerk/clerk-react` from frontend; stub deleted API clients
- [x] Remove Telemetry page and nav link
- [x] Rewrite `cmd/all-in-one/main.go` — management plane only
- [x] Simplify `internal/config/config.go`
- [x] Update `Dockerfile` — remove DuckDB/CGO complexity, drop OTLP ports
- [x] Update `docker-compose.yml` — rename services, remove OTLP
- [x] Create `otel-hive.yaml` (simplified default config)
- [x] Write `CLAUDE.md`, `README.md`
- [x] `go build ./...` passes, `npm run build` passes
- [x] Committed and pushed to `main`

---

## Phase 2 — Built-in Auth
> Target: next session

### Backend (`internal/auth/`, `internal/middleware/`)
- [ ] SQLite schema migration: `users(id, username, password_hash, role, created_at)`, `api_keys(id, user_id, key_hash, name, last_used)`
- [ ] `internal/auth/service.go`: BCrypt hash/verify, JWT HS256 sign/verify (15m access + 7d refresh), API key generate/validate
- [ ] `internal/middleware/auth.go`: Gin middleware — validate Bearer JWT or `X-API-Key` header
- [ ] Auth routes: `POST /api/auth/login`, `POST /api/auth/logout`, `POST /api/auth/refresh`, `GET/POST /api/auth/api-keys`
- [ ] First-run detection: no users → return 503 with `{"setup_required": true}` on all routes
- [ ] Apply auth middleware to all existing routes (agents, configs, groups)
- [ ] Add `golang-jwt/jwt` and `golang.org/x/crypto` to `go.mod`

### Frontend (`ui/src/`)
- [ ] `pages/Login.tsx`: email + password form, JWT stored in httpOnly cookie
- [ ] `pages/Setup.tsx`: first-run wizard — create admin account (3 steps)
- [ ] `pages/ApiKeys.tsx`: list + create + revoke API keys
- [ ] Auth guard: check session on app load; redirect unauthenticated users to `/login`
- [ ] Nav update: add user menu (logout, profile)
- [ ] Remove auth stubs from `api/` clients; add `Authorization` header to all requests

### Verification
- [ ] `go build ./...` passes
- [ ] `npm run build` passes
- [ ] All API routes return 401 without valid token
- [ ] Fresh container → setup wizard → admin created → login → agents visible
- [ ] API key generated → can call API with `X-API-Key: <key>` header

---

## Phase 3 — Git Sync Service
### Backend (`internal/gitsync/`)
- [ ] SQLite schema: `git_sources(id, name, repo_url, token_encrypted, branch, config_root, provider, poll_interval, last_sync_sha, last_sync_at, status)`
- [ ] Provider interface: `FetchTree(path)`, `FetchFile(path, sha)`, `GetLatestSha()`
- [ ] GitHub provider (REST API)
- [ ] GitLab provider (REST API)
- [ ] Gitea provider (GitHub-compatible API)
- [ ] Generic HTTP provider (raw URL fetch)
- [ ] Config matcher: `environments/{env}/{role}.yaml` → collector label rules
- [ ] Background polling goroutine with jitter (default 5 min)
- [ ] Webhook endpoint `POST /api/webhook/git` — HMAC-SHA256 validation
- [ ] Manual trigger `POST /api/git-sources/{id}/sync`

### Frontend
- [ ] `pages/GitSources.tsx`: CRUD for git sources, sync status, "Sync Now" button
- [ ] Last-synced timestamp + sync log display

---

## Phase 4 — Multi-Environment
- [ ] SQLite schema: `environments(id, name, slug, description, created_at)`
- [ ] `agents` table: add `environment_id` FK
- [ ] `configs` table: add `environment_id` FK
- [ ] `registration_tokens` table for auto-assigning env on collector first connect
- [ ] API: environment CRUD + scoped agent/config queries
- [ ] Frontend: environment switcher in nav, per-env config history

---

## Phase 5 — UX Polish & Templates
- [ ] Setup wizard (3-step: admin → git repo → verify first collector)
- [ ] Config template library (6 seed templates)
- [ ] Audit log SQLite table + paginated UI
- [ ] Inline YAML editor validation against OTel schema
- [ ] Collector status dashboard with health badges
- [ ] README: quick-start guide, architecture diagram, collector setup guide
- [ ] GitHub Actions: GHCR automated Docker image build on tag

---

## Roadmap (post-MVP)
- [ ] OTLP telemetry backend (all-in-one mode)
- [ ] PostgreSQL adapter for >500 collector fleets
- [ ] Kubernetes Helm chart + OpAMP Bridge support
- [ ] SAML/OIDC SSO
- [ ] Config diff view before push
- [ ] otel.guru tool page linking to this project
