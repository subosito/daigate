# Daigate

**The composable AI gateway** — OpenAI/Anthropic-compatible ingress, encrypted credential store, catalog routing, compile-time adapters.

**Module:** `github.com/subosito/daigate` · **Go:** 1.26

Operators run `daigate serve` with yaml config. Integrators embed `gateway` in Go. Clients point any OpenAI-compatible SDK at the data plane — they never see upstream provider keys.

---

## What ships

| Piece | Role |
|-------|------|
| **CLI** (`daigate`) | `serve`, `credential`, `keys`, `adapters`, `admin` |
| **Passthrough** | Relay chat, embeddings, image, speech, video to upstream OpenAI/Anthropic wires |
| **Credential store** | Encrypted sqlite (`broker.db`) + generic OAuth2 login/refresh |
| **Catalog** | `providers.yaml` — models, upstream pools, failover / round-robin |
| **Plugin hooks** | `RegisterCredentialBackend`, `RegisterAdminIssuer`, `adaptersdk` for operator binaries |

Stock CLI includes **passthrough only**. Translate adapters and extra backends link at compile time in custom operator binaries.

Adapters are **Go compile-time only** — no WASM, no runtime plugin download.

---

## Quick start

```bash
go build -o bin/daigate ./cmd/daigate
cp examples/{daigate.yaml,providers.yaml} .

export DAIGATE_BROKER_KEY="$(openssl rand -base64 32)"
export DAIGATE_ADMIN_TOKEN="$(openssl rand -hex 32)"

./bin/daigate serve --config daigate.yaml
# :9420 data · 127.0.0.1:9421 admin
```

Mint a gateway key and import an upstream credential:

```bash
./bin/daigate keys create --static --name default --config daigate.yaml
./bin/daigate credential import openai --api-key sk-… --config daigate.yaml
```

Call the gateway:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:9420/v1"
export OPENAI_API_KEY="sk-dg-…"

curl -fsS "$OPENAI_BASE_URL/chat/completions" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.4-mini","messages":[{"role":"user","content":"hi"}]}'
```

Use **catalog model ids** from `providers.yaml`, not raw upstream names.

---

## Two planes

| Plane | Default | Purpose |
|-------|---------|---------|
| **Data** | `:9420` | `/v1/chat/completions`, `/v1/messages`, media, … |
| **Admin** | `127.0.0.1:9421` | Credential admin, OAuth callback, gateway key mint |

Gateway keys (`sk-dg-…`) authenticate clients to daigate. Upstream secrets stay on the server and inject at forward time.

---

## Ingress (data plane)

| Modality | Path |
|----------|------|
| Chat (OpenAI) | `POST /v1/chat/completions` |
| Chat (Anthropic) | `POST /v1/messages` |
| Responses | `POST /v1/responses` |
| Embeddings | `POST /v1/embeddings` |
| Image | `POST /v1/images/generations`, `/edits` |
| Speech | `POST /v1/audio/speech` |
| Video | `POST /v1/videos/generations`, `GET /v1/videos/{id}` |
| Health | `GET /v1/healthz` |

---

## Development

```bash
just          # go vet + go test -race
just build    # bin/daigate
```

Starter config: [`examples/`](examples/). Full runbook and doc index: **[docs/README.md](docs/README.md)**.

| Doc | Topic |
|-----|--------|
| [ingress.md](docs/ingress.md) | Client SDK env vars, gateway keys |
| [runtime.md](docs/runtime.md) | CLI vs library, operator binary |
| [architecture.md](docs/architecture.md) | Planes, components |
| [catalog.md](docs/catalog.md) | `providers.yaml` |
| [auth.md](docs/auth.md) | Credentials, OAuth, encryption |
| [security.md](docs/security.md) | Threat model, admin API |

**Agents:** [AGENTS.md](AGENTS.md)