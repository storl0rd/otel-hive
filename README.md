# otel-hive

**OpAMP-based OTel Collector fleet management.** A hive brain for your OpenTelemetry Collectors — centralized config distribution, health monitoring, Git-based config sync, and an operator-friendly UI that ships as a single Docker container.

## What it does

| Feature | Detail |
|---------|--------|
| **Fleet visibility** | Real-time agent health, last-seen, status, and effective config across all collectors |
| **Remote configuration** | Push config changes to any collector or group instantly via [OpAMP](https://opentelemetry.io/docs/specs/opamp/) |
| **Git sync** | Store collector configs in Git; otel-hive polls for changes and distributes them automatically (GitHub, GitLab, Gitea, raw HTTP) |
| **Built-in auth** | Username/password login + API keys; JWT sessions; no external auth provider needed |
| **Audit log** | Full history of every write operation — who changed which config and when |
| **Webhook support** | Trigger instant syncs on `git push` via HMAC-validated webhook |

---

## Quick start — Docker Compose

```bash
# 1. Grab the compose file
curl -fsSL https://raw.githubusercontent.com/storl0rd/otel-hive/main/docker-compose.yml -o docker-compose.yml

# 2. Set required secrets (or put them in a .env file)
export JWT_SECRET="$(openssl rand -hex 32)"

# 3. Start
docker compose up -d

# 4. Open the UI → setup wizard creates your admin account
open http://localhost:8080
```

**docker-compose.yml** (full reference):

```yaml
services:
  otel-hive:
    image: ghcr.io/storl0rd/otel-hive:latest
    ports:
      - "8080:8080"    # UI + REST API
      - "4320:4320"    # OpAMP WebSocket
    volumes:
      - ./data:/app/data
    environment:
      JWT_SECRET: "${JWT_SECRET}"              # required
      # GIT_REPO_URL: "https://github.com/org/otel-configs"  # optional: auto-configure a git source
      # GIT_TOKEN: "${GIT_TOKEN}"              # optional: for private repos
```

> **First run:** otel-hive detects no users exist and serves a setup wizard at `/setup`. Create your admin account there — all API routes return `503 setup_required` until this is done.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                otel-hive  (single container)                      │
│                                                                   │
│  ┌────────────────────┐  ┌──────────────────┐  ┌──────────────┐  │
│  │  React UI + REST   │  │  OpAMP WebSocket  │  │  Git Sync    │  │
│  │  API  :8080 (Gin)  │  │  :4320           │  │  Worker      │  │
│  └────────┬───────────┘  └────────┬─────────┘  └──────┬───────┘  │
│           └──────────────────────▼────────────────────┘          │
│                      SQLite  ./data/app.db                        │
│         agents │ configs │ groups │ users │ git_sources │         │
│         api_keys │ audit_log │ git_source_file_shas               │
└──────────────────────────────────────────────────────────────────┘
         ↕ OpAMP WebSocket              ↕ HTTPS REST API
  OTel Collectors                  Git Repository
  (with OpAMP Supervisor           (GitHub / GitLab /
   OR K8s-native mode)              Gitea / HTTP)
```

**Single binary. Embedded SQLite. No external dependencies.** Suitable for fleets up to ~500 collectors; see [resource requirements](#resource-requirements) for sizing.

---

## Connecting collectors

### Option A — OpAMP Supervisor (recommended for bare-metal / VMs)

Each managed collector runs alongside the [OpAMP Supervisor](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/cmd/opampsupervisor). The Supervisor manages the collector as a subprocess and proxies OpAMP to otel-hive.

**1. Install the supervisor binary:**

```bash
# Example — replace VERSION and ARCH as needed
curl -fsSL https://github.com/open-telemetry/opentelemetry-collector-contrib/releases/download/vVERSION/opampsupervisor_linux_amd64 \
  -o /usr/local/bin/opampsupervisor
chmod +x /usr/local/bin/opampsupervisor
```

**2. Create `supervisor.yaml`:**

```yaml
server:
  endpoint: ws://otel-hive:4320/v1/opamp
  headers:
    Authorization: "Bearer YOUR_API_KEY"   # create in otel-hive UI → API Keys

agent:
  executable: /usr/bin/otelcol
  config_apply_timeout: 30s
  bootstrap_config_file: /etc/otelcol/bootstrap.yaml  # used before first remote config arrives

capabilities:
  accepts_remote_config: true
  reports_effective_config: true
  reports_health: true
```

**3. Run:**

```bash
opampsupervisor --config supervisor.yaml
```

The collector appears in otel-hive within seconds. You can now push configs from the UI or let Git sync handle it.

### Option B — Kubernetes (ConfigMap-based, no supervisor required)

For K8s workloads you can skip the supervisor entirely. otel-hive pushes configs via the OpAMP extension built into the collector itself, or you can use a simple GitOps loop:

```yaml
# collector-deploy.yaml (simplified)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: otelcol
spec:
  template:
    spec:
      containers:
        - name: otelcol
          image: otel/opentelemetry-collector-contrib:latest
          args: ["--config=/conf/config.yaml"]
          volumeMounts:
            - name: config
              mountPath: /conf
      volumes:
        - name: config
          configMap:
            name: otelcol-config   # otel-hive updates this ConfigMap on sync
```

> K8s-native mode (ConfigMap update + rollout restart) is on the roadmap. Until then, use the OpAMP extension or the supervisor approach above.

---

## Git config sync

Store your collector configs in a Git repository. otel-hive polls for changes and pushes them to matching agents automatically.

### Repository layout

```
your-configs-repo/
└── configs/                        ← config_root (default)
    ├── environments/
    │   ├── production/
    │   │   ├── gateway.yaml        → agents with env=production, role=gateway
    │   │   └── node-agent.yaml     → agents with env=production, role=node-agent
    │   ├── staging/
    │   │   └── gateway.yaml        → agents with env=staging, role=gateway
    │   └── default/
    │       └── base.yaml           → all agents (fallback)
    └── groups/
        └── aws-east.yaml           → agents with group.id=aws-east
```

Path segments map directly to agent labels. A collector is matched when **all** label selectors in the file's path match its registered labels.

### Setting up a git source

1. Go to **Git Sources** in the sidebar → **Add Source**
2. Fill in: provider, repo URL, access token (for private repos), branch, config root
3. Set a poll interval (default: 5 minutes)
4. Optionally add a webhook secret for instant push-triggered syncs

### Webhook setup (GitHub example)

```
URL:    https://your-otel-hive/api/webhook/git/<source-id>
Content-Type: application/json
Secret: <your webhook secret>
Events: Push
```

---

## API keys

Collectors and CI/CD pipelines authenticate with API keys (prefix `ohk_`). Create them in the UI under **API Keys** or via the REST API:

```bash
curl -X POST https://your-otel-hive/api/auth/api-keys \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "ci-pipeline"}'
```

---

## REST API

All endpoints (except `/health`, `/metrics`, auth setup/login, and webhook) require `Authorization: Bearer <token>` or `X-API-Key: ohk_<key>`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/agents` | List all agents |
| `POST` | `/api/v1/agents/:id/config` | Push config to agent |
| `POST` | `/api/v1/agents/:id/restart` | Restart agent |
| `GET/POST/PUT/DELETE` | `/api/v1/configs` | Manage config library |
| `GET/POST/PUT/DELETE` | `/api/v1/groups` | Manage agent groups |
| `GET/POST/PUT/DELETE` | `/api/v1/git-sources` | Manage git sources |
| `POST` | `/api/v1/git-sources/:id/sync` | Trigger manual sync |
| `GET` | `/api/v1/audit-log` | Paginated audit log |
| `GET` | `/metrics` | Prometheus metrics |

---

## Resource requirements

| Fleet size | vCPU | RAM | Disk | Notes |
|-----------|------|-----|------|-------|
| Lab / dev (< 20) | 0.25 | 128 MB | 2 GB | Single container, SQLite |
| Small (20–100) | 0.5 | 256 MB | 10 GB | Comfortable baseline |
| Medium (100–300) | 1 | 512 MB | 20 GB | WAL mode handles concurrency |
| Large (300–500) | 2 | 1 GB | 50 GB | Monitor SQLite write latency |
| > 500 | — | — | — | PostgreSQL adapter planned |

---

## Development

```bash
# Backend
go run ./cmd/all-in-one --config ./otel-hive.yaml

# Frontend (separate terminal)
cd ui && npm install && npm run dev    # Vite dev server at :5173

# Run tests
go test ./internal/... ./cmd/... ./integration/...
```

**Config file** (`otel-hive.yaml`):

```yaml
server:
  http_port: 8080
  opamp_port: 4320

auth:
  jwt_secret: "change-me"    # or set JWT_SECRET env var
  access_token_expiry: "15m"
  refresh_token_expiry: "168h"

database:
  path: "./data/app.db"

logging:
  level: "info"
  format: "json"
```

---

## Roadmap

- [x] OpAMP server — agent health, remote config, groups
- [x] Built-in auth — username/password + JWT + API keys
- [x] Git config sync — GitHub, GitLab, Gitea, raw HTTP; poll + webhook
- [x] Audit log — paginated event history
- [ ] Config template library (6 starter templates)
- [ ] K8s-native config apply (ConfigMap + rollout)
- [ ] PostgreSQL adapter for > 500 collector fleets
- [ ] Helm chart
- [ ] SAML/OIDC SSO
- [ ] All-in-one telemetry backend (DuckDB / ClickHouse)

---

## License

Apache 2.0 — see [LICENSE](LICENSE).

Built by [Vaishak Nair](https://github.com/storl0rd) · [otel.guru](https://otel.guru)
