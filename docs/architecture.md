# Architecture

## Two planes

daigate separates **credential control** from **compatible ingress**:

```text
┌──────────────────────────────────────────────────────────────────┐
│                         daigate process                         │
│                                                                   │
│  ┌─────────────────────┐      ┌─────────────────────────────┐  │
│  │  CONTROL PLANE       │      │  DATA PLANE                  │  │
│  │  (credential admin)  │      │  (compatible ingress API)    │  │
│  │                      │      │                              │  │
│  │  • Store CRUD        │      │  • Client API key auth       │  │
│  │  • OAuth login flow  │◄────►│  • Catalog resolve           │  │
│  │  • Snapshot          │      │  • Inject upstream creds      │  │
│  │  • Refresh loop      │      │  • Upstream HTTPS relay       │  │
│  │  • Issued API keys   │      │  • Ingress observability     │  │
│  └─────────────────────┘      └─────────────────────────────┘  │
│         :9421 admin (opt)              :9420 data (default)       │
└──────────────────────────────────────────────────────────────────┘
```

| Plane | Default port | Packages |
|-------|--------------|----------|
| Control | `:9421` | `credential/admin`, `ingress/adminauth` |
| Data | `:9420` | `gateway/`, `wire/`, `upstream/`, `catalog/` |

---

## Product decision: Go adapters only (no WASM)

**WASM is out of scope.** All adapters are **trusted Go**, compiled into the operator binary at build time.

| Path | When |
|------|------|
| **Core CLI** (`daigate`) | `passthrough` only — enough for OpenAI/Anthropic-compat vendors via `providers.yaml` |
| **Custom operator binary** | Link credential backends, issuers, translate adapters at compile time |
| **Custom adapter** | New Go module + `adaptersdk` + `compose` in your binary |

No runtime plugin loading, no `.wasm`, no `.so`.

---

## What ships

### Core (`github.com/subosito/daigate`)

| Component | Package | Role |
|-----------|---------|------|
| Gateway engine | `gateway/` | Assemble listeners, lifecycle, `OpenStore` |
| Ingress wires | `wire/` | Route `/v1/*` to catalog + adapter handlers |
| Catalog | `catalog/` | Load `providers.yaml`; resolve model → upstream target; pool strategies (`failover`, `round_robin`) |
| Passthrough adapter | `passthrough/` | Protocol relay (chat, embed, image, speech, video) |
| Adapter SDK | `adaptersdk/` | Public API for adapter authors |
| CLI compose | `compose/` | Filter `adapters.enable` → registry (passthrough default) |
| Upstream relay | `upstream/` | HTTPS forward after inject |
| Credential store | `credential/store/` | `Store` interface; sqlite (+ memory for tests) |
| Encryption | `credential/seal/` | AES-256-GCM at rest |
| Inject | `credential/inject/` | Bearer, `x-api-key`, OAuth presets |
| Generic OAuth | `credential/oauth/generic/` | Stock OAuth2 login/refresh |
| Admin HTTP | `credential/admin/` | Credential CRUD, snapshot, provision routes |
| Gateway keys | `ingress/keyring/` | Argon2id hash verify + admin key CRUD |
| Admin auth | `ingress/adminauth/` | Admin vs provision tokens |
| Argon helper | `ingress/argonhash/` | Hashing for keyring |
| Observability | `observability/` | Ingress stderr JSON always; OTel when standalone `Boot` or library `Hook` ([observability.md](observability.md)) |
| Config | `internal/config/` | `daigate.yaml` loader — generic plugin slots (`IssuerEntry`, `backend_config`) |
| CLI | `cmd/daigate/` | `serve`, `credential`, `keys`, `adapters`, `admin` |
| Plugin hooks | `gateway/backend.go`, `gateway/admin_issuer.go` | `RegisterCredentialBackend`, `RegisterAdminIssuer` — linked at compile time |

### Not in stock CLI — operator choice

| Item | Where | Notes |
|------|-------|-------|
| Vendor `oauth.Module` (Anthropic, Codex, …) | Operator `main.go` at build time | Generic OAuth2 in core is enough for most vendors |
| OAuth inject presets (`anthropic_oauth`, …) | Operator binary | `inject.RegisterOAuthPreset` |
| Translate adapters (`myvendor`, …) | Operator binary via `adaptersdk` | Shape conversion when upstream wire differs |
| Alternate credential backends | Operator binary | `gateway.RegisterCredentialBackend` decodes `backend_config` |
| Self-service key issuers | Operator binary | `gateway.RegisterAdminIssuer` decodes `ingress.issuers[].config` |

---

## Should auth and router stay split?

### Option A — One process, two listeners (recommended default)

One `daigate serve` binary:

- **Data** `:9420` — LLM wires; authenticated via **`keyring`** (hash lookup; `static` or `issued` keys).
- **Admin** `:9421` — credential admin, OAuth callback, snapshot, optional key issuance.

**Library embedding:** `gateway.New(gateway.Config{Store: memStore})` — no admin listener; credentials via `Store` interface only.

### Option B — Two processes (optional deployment)

Hard credential-store isolation for hosted multi-tenant — operational overhead; not default.

---

## Component diagram

```text
                    ingress.ClientAuth (keyring)
                           │
  Client ──► wire.Handler ─┼─► catalog.Resolve(model, wire)
                           │         │
                           │         ▼
                           │    catalog.ProviderPool
                           │         │
                           ▼         ▼
              adaptersdk handler ──► credential.Store.Get(profile)
              (compiled registry)      │
                           │    inject.Apply (api_key, oauth preset, …)
                           ▼
                      upstream HTTPS
```

Operator modules register at link time:

```text
  linked backend plugin   ──►  gateway.RegisterCredentialBackend(name, …)
  linked issuer plugin    ──►  gateway.RegisterAdminIssuer(driver, …)
  operator adapters/      ──►  translate handlers via adaptersdk + compose
```

---

## Request lifecycle (chat)

1. **Ingress auth** — validate client Bearer against `keyring`.
2. **Wire select** — path determines wire id.
3. **Catalog resolve** — `model` + wire → modality → provider surface → upstream model string.
4. **Pool strategy** — `failover` (ordered retry) / `round_robin` (load spread).
5. **Credential load** — `Store.Get(credential_profile)`.
6. **Inject** — strip client auth; apply upstream headers.
7. **Forward** — stream SSE; `observability` records one ingress line per request (+ span/metrics when `Boot` or `Hook` active).

**OTel lifecycle:** standalone `main` calls `Boot` before `ListenAndServe`; library embedders call `Hook` after host `Boot`. `gateway.ListenAndServe` does not init or shutdown exporters.

**Chat** relays passthrough when `surface.protocol` matches the ingress wire; operator-linked translate adapters handle vendor-specific APIs. **Media** translate adapters convert shapes inside the handler ([media.md](media.md)).

CLI vs library → [runtime.md](runtime.md). Adapter authoring → [adapters.md](adapters.md), [adaptersdk.md](adaptersdk.md).