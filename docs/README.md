# daigate documentation

Composable AI gateway — compatible ingress, pluggable credentials, catalog routing, compile-time adapters.

**Module:** `github.com/subosito/daigate`

Core ships **passthrough** + encrypted sqlite credentials (`broker.db`). Credential backends, issuers, and translate adapters are **operator binary** choices — link plugins at compile time via `gateway.RegisterCredentialBackend`, `gateway.RegisterAdminIssuer`, and `adaptersdk`. Starter config: [`examples/`](../examples/).

**Adapters are Go compile-time only** — no WASM, no runtime plugins.

---

## Run locally

Inside `devenv shell` (or with Go 1.26+ on your PATH):

**1. Build the CLI**

```bash
go build -o bin/daigate ./cmd/daigate
# or: just build
```

**2. Config and secrets**

```bash
cp examples/daigate.yaml daigate.yaml
cp examples/providers.yaml providers.yaml
# edit providers.yaml if your upstream base_url or models differ

export DAIGATE_BROKER_KEY="$(openssl rand -base64 32)"
export DAIGATE_ADMIN_TOKEN="$(openssl rand -hex 32)"
```

**3. Start the gateway** (terminal 1)

```bash
./bin/daigate serve --config daigate.yaml
# listens :9420 (data) and 127.0.0.1:9421 (admin) by default
```

**4. Mint a gateway key and wire upstream credentials** (terminal 2)

```bash
./bin/daigate keys create --static --name default --config daigate.yaml
# copy sk-dg-… from output

./bin/daigate credential import openai --api-key sk-your-upstream-key --config daigate.yaml
# profile name must match providers.*.credential_profile in providers.yaml
```

**5. Call the data plane**

```bash
export OPENAI_BASE_URL="http://127.0.0.1:9420/v1"
export OPENAI_API_KEY="sk-dg-…"

curl -fsS "$OPENAI_BASE_URL/chat/completions" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.4-mini","messages":[{"role":"user","content":"hi"}]}'
```

Clients use **catalog model ids** from `providers.yaml` (`gpt-5.4-mini`, …), not raw upstream names.

**Translate adapters:** operator binary via `adaptersdk` — not in stock CLI.

---

## Dev

```bash
just          # go vet + go test -race ./...
just build    # compile bin/daigate
```

CI runs the same checks on push/PR ([`.github/workflows/ci.yml`](../.github/workflows/ci.yml)). Integration coverage lives in `go test ./...` — no separate smoke script.

### Integrator (library)

```go
gateway.New(gateway.Config{Adapters: …, CatalogPath: "providers.yaml", Store: store})
```

**Observability:** if your binary already exports OTel, call `observability.Hook("daigate")` after your `Boot` — do not call daigate `Boot` inside `ListenAndServe`. Standalone operator binaries call `Boot` + `ShutdownGraceful` in `main`. → [observability.md](observability.md)

→ [runtime.md](runtime.md) — CLI vs library, operator binary, adapter loading

---

## Listeners & ports

Default **Option A** — one process, two listeners ([architecture.md](architecture.md)):

| Plane | Default bind | Config | Purpose |
|-------|--------------|--------|---------|
| **Data** | `:9420` | `serve.data_listen` | OpenAI/Anthropic-compatible API (`/v1/chat/completions`, `/v1/messages`, media, …) |
| **Admin** | `127.0.0.1:9421` | `admin.listen` | Credential admin, OAuth callback, gateway key mint |

```yaml
serve:
  data_listen: "127.0.0.1:9420"   # prod: bind loopback; edge proxy terminates TLS
  catalog: providers.yaml

admin:
  listen: "127.0.0.1:9421"        # keep loopback in prod unless behind trusted proxy
```

**Client base URL must include `/v1`:**

```bash
export OPENAI_BASE_URL="http://127.0.0.1:9420/v1"
export OPENAI_API_KEY="sk-dg-…"    # gateway key — not an upstream provider key
```

**CI smoke** mints keys on the admin plane, calls the data plane:

| Env | Example | Plane |
|-----|---------|-------|
| `OPENAI_BASE_URL` | `http://127.0.0.1:9420/v1` | Data |
| `DAIGATE_ADMIN_URL` | `http://127.0.0.1:9421` | Admin |
| `DAIGATE_PROVISION_TOKEN` | provision secret | Admin auth for `POST /v1/keys` |

**Required secrets** (see [security.md](security.md)):

| Env | Role |
|-----|------|
| `DAIGATE_BROKER_KEY` | AES key for encrypted `broker.db` (`DAIGATE_VAULT_KEY` legacy alias) |
| `DAIGATE_ADMIN_TOKEN` | Full admin plane |
| `DAIGATE_PROVISION_TOKEN` | CI key mint only (optional until CI) |

---

## Ingress

| Modality | Path |
|----------|------|
| Chat (OpenAI) | `POST /v1/chat/completions` |
| Chat (Anthropic) | `POST /v1/messages` |
| Responses | `POST /v1/responses` |
| Embeddings | `POST /v1/embeddings` |
| Image | `POST /v1/images/generations`, `/edits` |
| Speech | `POST /v1/audio/speech` |
| Video | `POST /v1/videos/generations`, `GET /v1/videos/{id}` |

→ [ingress.md](ingress.md) for SDK env vars, CI provisioning, and client integration

---

## Docs (read order)

| # | Doc | Topic |
|---|-----|--------|
| 1 | [ingress.md](ingress.md) | SDK env vars, gateway keys — **start here for client integration** |
| 2 | [runtime.md](runtime.md) | **CLI vs library**, operator binary, adapter loading |
| 3 | [architecture.md](architecture.md) | Planes, what ships in core |
| 4 | [catalog.md](catalog.md) | `providers.yaml`, wires |
| 5 | [adapters.md](adapters.md) | Core `passthrough` + operator translate adapters |
| 6 | [adaptersdk.md](adaptersdk.md) | SDK for external adapter authors |
| 7 | [media.md](media.md) | Image / speech / video |
| 8 | [observability.md](observability.md) | Ingress logs, OTel env, collector |
| 9 | [auth.md](auth.md) | Ingress/egress interfaces, OAuth, encryption setup, gateway keys |
| 10 | [security.md](security.md) | Threat model, admin API, credential listing rules |

---

## Layout

```text
daigate/                    # core — passthrough, gateway, encrypted broker sqlite
  cmd/daigate/              # CLI (serve, credential, keys, …)
  gateway/  wire/  catalog/  upstream/  credential/
  adaptersdk/  passthrough/
  compose/  observability/
  examples/                 # starter daigate.yaml + providers.yaml (copy to repo root)
  testdata/fixtures/        # mock providers.yaml for unit tests only
  docs/                     # operator + integrator documentation
```

---

## Glossary

| Term | Meaning |
|------|---------|
| **Ingress** | Customer `/v1/*` paths on data plane `:9420` |
| **Admin plane** | Credential + key control on `:9421` |
| **Wire** | Ingress contract id |
| **Protocol** | Passthrough handler on surface (`openai-chat-completions`, …) |
| **Adapter** | Translate handler on surface (operator Go code, e.g. `myvendor`) |
| **Gateway key** | Client auth to daigate (`sk-dg-…`) |
| **adaptersdk** | Public API for adapter authors |
| **broker key** | `DAIGATE_BROKER_KEY` — AES key for encrypted `broker.db` |
| **backend_config** | Opaque yaml per `credential.backend` — decoded by linked backend plugin |
| **Boot** | Standalone OTel init (`observability.Boot`) — daigate owns OTLP export |
| **Hook** | Library OTel attach (`observability.Hook`) — embedder owns OTLP export |