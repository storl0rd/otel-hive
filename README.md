# otel-hive

**OpAMP-based OTel Collector fleet management.** A hive brain for your OpenTelemetry Collectors — centralized config distribution, health monitoring, Git-based config sync, and a junior-operator-friendly UI.

> Forked from [Lawrence OSS](https://github.com/getlawrence/lawrence-oss) (Apache 2.0). Stripped to a management plane and extended with Git sync, built-in auth, and multi-environment support.

## What it does

- **Fleet visibility** — real-time agent health, status, and effective config across all collectors
- **Remote configuration** — push config changes to any collector or group instantly via [OpAMP](https://opentelemetry.io/docs/specs/opamp/)
- **Git sync** — store collector configs in Git; otel-hive polls for changes and distributes them automatically (GitHub, GitLab, Gitea, raw HTTP)
- **Multi-environment** — manage prod/staging/dev collector fleets with isolated config scopes
- **Audit log** — full history of who changed what config and when
- **Built-in auth** — username/password + API keys; no external auth provider required

## Quick start

```bash
# 1. Copy config and start
cp otel-hive.yaml.example otel-hive.yaml   # edit as needed
docker compose up -d

# 2. Open UI
open http://localhost:8080
```

On first run, a setup wizard creates your admin account.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                  otel-hive  (:8080 + :4320)                   │
│                                                               │
│  React UI + REST API │ OpAMP WebSocket │ Git Sync Worker      │
│  ─────────────────────────────────────────────────────────── │
│                     SQLite (./data/app.db)                    │
└──────────────────────────────────────────────────────────────┘
        ↕ OpAMP                             ↕ HTTPS
  OTel Collectors                     Git Repository
  (+ OpAMP Supervisor)                (GitHub/GitLab/etc.)
```

Single binary. No external database. Runs anywhere Docker runs.

## Collector setup

Each managed collector needs the [OpAMP Supervisor](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/cmd/opampsupervisor) installed alongside it.

```yaml
# supervisor.yaml
server:
  endpoint: ws://otel-hive:4320/v1/opamp
  headers:
    Authorization: "Bearer YOUR_API_KEY"

agent:
  executable: /usr/bin/otelcol
  config_apply_timeout: 30s

capabilities:
  accepts_remote_config: true
  reports_effective_config: true
  reports_health: true
```

## Git config layout

```
your-configs-repo/
└── configs/
    ├── environments/
    │   ├── production/
    │   │   ├── gateway.yaml       # → collectors: env=production, role=gateway
    │   │   └── node-agent.yaml    # → collectors: env=production, role=node-agent
    │   ├── staging/
    │   │   └── gateway.yaml
    │   └── default/
    │       └── base.yaml          # fallback
    └── groups/
        └── aws-east.yaml          # → collectors: region=aws-east
```

## Resource requirements

| Fleet size | vCPU | RAM | Disk |
|-----------|------|-----|------|
| < 20 collectors | 0.25 | 128 MB | 2 GB |
| 20–100 | 0.5 | 256 MB | 10 GB |
| 100–300 | 1 | 512 MB | 20 GB |
| 300–500 | 2 | 1 GB | 50 GB |

## Development

```bash
# Backend (Go)
go run ./cmd/all-in-one --config ./otel-hive.yaml

# Frontend (React + Vite)
cd ui && npm install && npm run dev
```

## Roadmap

- [x] OpAMP server (agent health, remote config, groups)
- [ ] Built-in auth (username/password + API keys)
- [ ] Git config sync (GitHub, GitLab, Gitea, raw HTTP)
- [ ] Multi-environment support
- [ ] Audit log
- [ ] Config template library
- [ ] Setup wizard
- [ ] Helm chart for Kubernetes
- [ ] All-in-one telemetry backend (roadmap)

## License

Apache 2.0 — see [LICENSE](LICENSE).

Built on [otel.guru](https://otel.guru) by [Vaishak Nair](https://github.com/storl0rd).
