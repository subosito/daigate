# AGENTS.md — daigate

`github.com/subosito/daigate` — composable AI gateway (Go 1.26).

## Repo

Dual-plane ingress, encrypted sqlite credential store (`broker.db`), catalog routing (`providers.yaml`), compile-time adapters.

| Plane | Default | Role |
|-------|---------|------|
| Data | `:9420` | OpenAI/Anthropic-compatible `/v1/*` |
| Admin | `:9421` | Credentials, OAuth, gateway keys |

Stock `cmd/daigate` ships **passthrough** only. Operator binaries link extra adapters and plugins at compile time.

## Layout

```text
cmd/daigate/       # serve, credential, keys, adapters, admin
gateway/           # listeners, OpenStore, plugin registries
wire/ catalog/     # routing, providers.yaml
passthrough/       # protocol relay (stock CLI adapter)
adaptersdk/        # adapter author API
compose/           # DefaultAdapters(), FromConfig()
credential/        # store, seal, inject, oauth, admin HTTP
ingress/           # keyring, adminauth
observability/     # ingress logs, OTel Boot/Hook
examples/          # starter daigate.yaml, providers.yaml
docs/              # documentation index: docs/README.md
```

## Catalog modality hints

**Modality `<key>` names are operator-defined** in `providers.yaml` (`map[string]Modality`). daigate does not ship a fixed enum. Wire-implied hint today: `embed` on `openai-embeddings`. Everything else is yaml — hosts may add keys like `search_web`; those are **not** daigate built-ins.

When one model + wire maps to multiple rows (e.g. `chat` + `image` on `openai-chat-completions`), set **`X-Catalog-Modality`** to the yaml key. `catalog.ModalityHintFromRequest` → `ResolveWithModality`; header stripped upstream. Exception: `chat` + `search_web`/`search_x` only → defaults to `chat` (`pickModality`).

Hosts shape headers via `gateway.WrapDataHandler`. `gateway.Profile` exported for credential CLIs.

## Plugin hooks

Core registers plugins via link-time hooks (implementations live in operator modules):

| API | File |
|-----|------|
| `gateway.RegisterCredentialBackend` | `gateway/backend.go` |
| `gateway.RegisterAdminIssuer` | `gateway/admin_issuer.go` |
| `compose.FromConfig` + `adaptersdk` | translate adapters at serve time |

`credential.backend` + `backend_config` and `ingress.issuers[].config` are opaque yaml — decoded by linked plugins.

## Terms

| Term | Meaning |
|------|---------|
| Credential store | `broker.db` (encrypted sqlite) |
| broker key | `DAIGATE_BROKER_KEY` |
| Protocol | Passthrough surface handler |
| Adapter | Translate surface handler |
| Wire | Catalog ingress contract id |

## Commands

```bash
just          # go vet + go test -race ./...
just build    # bin/daigate
```

## Making changes

- **Passthrough protocols** → `passthrough/`
- **Stock CLI adapters** → `compose.DefaultAdapters()` (passthrough only)
- **Config** → `examples/` for starters; document in `docs/`
- **Admin security** → metadata-only credential list; provision token cannot read upstream credentials
- **Docs** → edit the relevant `docs/*.md`; update `docs/README.md` index when adding pages

## Verify

```bash
just
```