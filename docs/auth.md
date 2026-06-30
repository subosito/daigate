# Auth

Auth in daigate is **two different problems**. Both are plugin-based.

```text
  (A) INGRESS — who may call daigate?
  (B) EGRESS  — how daigate authenticates to Anthropic/OpenAI/…
```

Do not conflate them. Client API keys are **not** provider API keys.

---

## (A) Ingress — gateway client authentication

**Question:** How do *your* clients (SDKs, workers, SaaS tenants) prove identity to `:9420`?

Gateway keys are **not** upstream provider secrets. All gateway keys share one verify path — **argon2id hash lookup** in `KeyStore`. No env literal shortcut, no separate verify drivers per key type.

### Verify — `ingress.Authenticator` (`:9420`)

```go
type Authenticator interface {
    Authenticate(ctx context.Context, r *http.Request) (Principal, error)
}

type Principal interface {
    ID() string
}
```

Ships one implementation — **`keyring`**: hash incoming `Authorization: Bearer` → lookup row → check `revoked`, `expires_at`, and optional **gateway key scopes** (`keys create --scopes`, `wire:` / `model:` filters on forward and `GET /v1/models`). OAuth profile `scopes` in yaml are unrelated — they configure the authorization server.

```yaml
ingress:
  client_auth: keyring    # only verify mode in v1 — all keys, same code path
```

### Key kinds (metadata — not verify drivers)

Both kinds are gateway keys in the same table, same hash verify:

| Kind | Meaning | `expires_at` | Typical create path |
|------|---------|--------------|---------------------|
| **`static`** | Long-lived operator key | `null` | `keys create --static` |
| **`issued`** | Named / tenant / CI key | set | admin, **provision**; optional issuer plugins |

```bash
# static — solo dev or single-app deploy; secret shown once, hash in DB
daigate keys create --static --name default
# → sk-dg-…  kind=static  expires_at=null

# issued — app or tenant with TTL
daigate keys create --name my-app --ttl 720h
# → sk-dg-…  kind=issued  expires_at=…
```

**No `DAIGATE_STATIC_KEY`.** “Static” names the **key kind**, not an env-based verify shortcut.

### Issue — how keys enter `KeyStore`

| Mechanism | Creates kind | Auth | Can list upstream credentials? |
|-----------|--------------|------|---------------------------|
| `daigate keys create --static` | `static` | admin (CLI local or token) | Admin CLI yes |
| `POST /v1/keys` (admin listener) | `issued` or `static` | **admin** or **provision** | Admin yes; provision **no** (`/v1/credentials` denied). Provision mints `issued` only, scopes from `admin.provision`. |
Self-service issuers mount on the admin listener via `gateway.RegisterAdminIssuer` — linked in the operator binary.

**Token format at issue:** prefixed `sk-dg-…` (default).

### Control-plane auth tiers (admin vs provision)

Two bearer tokens for `:9421`: **admin** for credential/OAuth management; **provision** for CI gateway-key mint only. Role matrix, routes, and yaml: [security.md § Admin authentication](security.md#admin-authentication), [security.md § Admin API](security.md#admin-api). Starter config: [examples/daigate.yaml](../examples/daigate.yaml).

Provisioner always creates **`issued`** keys (TTL required). CI mints via `POST /v1/keys` on the admin listener — [ingress.md § CI/CD](ingress.md#cicd--smoke-tests).

```bash
daigate admin token create --role provision --name github-actions
```

### Defaults

| Deployment | Static key | Issued keys |
|------------|------------|-------------|
| **Solo dev** | `keys create --static` once | — |
| **Prod apps** | optional one `static` for internal tools | per tenant / service |
| **CI smoke** | — | provision mints `issued` per run |

---

## (B) Egress — upstream credential storage

**Question:** Where are upstream provider secrets stored and how are they refreshed?

### Store interface

```go
type Store interface {
    Get(ctx context.Context, profile string) (Material, error)
    PutAPIKey(ctx context.Context, profile, key string) (int64, error)
    PutOAuth(ctx context.Context, profile string, m Material) (int64, error)
    ListSummaries(ctx context.Context) ([]CredentialSummary, error)
    SnapshotMeta(ctx context.Context) (SnapshotMeta, error)
    BumpGeneration(ctx context.Context) error
    // …
}
```

### Store drivers (core)

| Driver | Use |
|--------|-----|
| `memory` | Tests, CI |
| `sqlite` | Single-node prod (`broker.db`) — **default** |

`credential.backend` + opaque `backend_config` are plugin hooks — decoded by whichever backend module is linked in the operator binary. Stock CLI uses encrypted sqlite only (`backend: sqlite` or default).

Store holds **upstream** credentials keyed by `credential_profile` (e.g. `openai`, `anthropic`, `acme`). Forward path: in-process `Get` → inject; admin HTTP never returns secret values ([security.md § Credential listing](security.md#credential-listing-operator)).

### Encryption at rest (required)

Secrets in `broker.db` (and any persistent `KeyStore` tables) are **never stored as plaintext JSON**. Opening the SQLite file or copying `broker.db` must not reveal API keys, access tokens, or refresh tokens.

| Layer | Rule |
|-------|------|
| **Write path** | `Store.Put` encrypts material before INSERT/UPDATE |
| **Read path** | `Store.Get` decrypts in process memory only |
| **On disk** | Ciphertext + nonce per row (AES-256-GCM) |
| **Memory store** | Same encryption in prod; `encryption: disabled` allowed **tests/CI only** |
| **Operator list / HTTP** | Metadata only — never api_key, access, or refresh ([security.md](security.md)) |
| **Forward path** | `Store.Get` in-process; secrets never on admin HTTP |

**Master key** — operator-supplied, outside the DB:

```yaml
# daigate.yaml
credential:
  broker: broker.db
  encryption:
    key_env: DAIGATE_BROKER_KEY   # 32-byte secret, base64 — preferred prod
    # or key_file: /etc/daigate/broker.key  (0600, not in git)
```

```bash
# generate once, store in systemd EnvironmentFile / secret manager
openssl rand -base64 32
```

| Concern | Approach |
|---------|----------|
| Algorithm | AES-256-GCM; random 12-byte nonce per encrypt |
| Column shape | `credentials.data` = JSON envelope `{"v":1,"alg":"aes-256-gcm","nonce":"…","ct":"…"}` |
| Key rotation | Single `DAIGATE_BROKER_KEY` per deployment; document backup before rotation |
| No key at startup | **Fail closed** — `serve` and `credential login` refuse to open encrypted store |
| Ingress gateway keys | Store **hash** (argon2id) for verification — not reversible secrets in DB |

**Threat model:** protects against casual DB theft, backups, and `sqlite3 broker.db` inspection. Does not protect a compromised host with the key in env — that requires HSM/KMS.

```text
  login/import ──► encrypt(plaintext) ──► broker.db (ciphertext only)
  forward path   ◄── decrypt(ciphertext) ◄── Store.Get (in-memory plaintext, never logged)
```

### Inject appliers

After `Store.Get`, `inject.Applier` builds outbound headers:

| Material type | Typical inject |
|---------------|----------------|
| `api_key` | `Authorization: Bearer …` or `x-api-key` |
| `oauth_access` | Bearer + vendor-specific headers |
| Custom preset | Vendor-specific inject presets (integrator binary) |

```go
type Applier interface {
    Apply(m Material, req *http.Request, spec ProviderSpec) error
}
```

Provider **templates** in catalog may declare:

- **`inject:`** — map of one or more headers with `${key}` / `${access}` / `${accountId}` / `${projectId}` placeholders (preferred for multi-header OAuth)
- **`inject_preset`** — shorthand for `bearer` or `x-api-key`

Resolution: `inject` map → `inject_preset` → adapter default → bearer. Full examples: [catalog-inject.md](catalog-inject.md).

OAuth extras (e.g. a `vendor_oauth` preset adding a beta header) register via `inject.RegisterOAuthPreset` in the **operator binary** at link time — not in stock CLI.

### Credential metadata (`Material.Extras`)

**daigate core is provider-agnostic.** The credential store may attach opaque key-value metadata on OAuth (and future kinds) as `extras` in the encrypted JSON blob. Core `inject.Apply` only handles generic auth shapes (Bearer, `x-api-key`, named header presets, registered OAuth presets).

| Layer | Responsibility |
|-------|----------------|
| **Core store** | Persist `extras` map; merge legacy `accountId` / `project_id` keys into `extras` on read for backward compat |
| **Core inject** | Never set vendor-specific headers from hard-coded field names |
| **Integrator adapters** | Read `Material.Extra("…")` and set upstream headers their vendor requires (e.g. `x-account-id` from `account_id`) |
| **Vendor OAuth modules** | Populate `extras` at login/refresh time; refresh preserves keys via `MergeExtras` |

Do **not** add per-provider fields to `store.Material` or core inject. New vendor requirements → new `extras` keys + adapter or `RegisterOAuthPreset` in the operator binary.

---

## (B) OAuth — generic host module (default)

Most upstream APIs use **standard OAuth 2.0 authorization code + PKCE**. daigate ships one host implementation — operators declare endpoints in `daigate.yaml` `credential_profiles`. Adapters never embed OAuth config or touch `broker.db`.

```yaml
# daigate.yaml or providers.yaml — profile id matches providers.*.credential_profile
credential_profiles:
  my-oauth:
    kind: oauth
    oauth:
      authorize_url: https://auth.example/oauth/authorize
      token_url: https://auth.example/oauth/token
      scopes: [openid]
      pkce: true
    inject: bearer                    # or inject_preset for non-Bearer vendors
```

```bash
daigate credential login my-oauth
daigate credential login my-oauth --flow=device    # SSH / headless
daigate credential login my-oauth --flow=auto      # browser if TTY, else device
```

### Login flows (`--flow`)

| Flow | When | UX |
|------|------|-----|
| **`auto`** (default) | Most cases | Browser if interactive terminal; else **device** (no hang on SSH) |
| **`browser`** | Local dev | PKCE + loopback callback on admin `:9421` |
| **`device`** | Headless / SSH / CI operator | Device code (like `gh auth login`) — no localhost redirect |
| **`manual`** | Broken browser | Print URL; operator pastes code |

Host owns token exchange, refresh loop, `broker.db` writes. Adapters only receive injected material via `handler.Target` — they never see refresh tokens.

### What generic OAuth2 covers

| Step | Host |
|------|------|
| Authorize (browser, device, or manual) | `credential/oauth/generic` |
| PKCE + loopback callback | `browser` flow + `credential/admin` |
| Device code poll | `device` flow when profile supports device endpoints |
| Code → access + refresh token | token endpoint POST |
| Periodic refresh | single-owner background loop ([below](#refresh-ownership-hard-rule)) |
| Outbound `Authorization` header | `inject.Applier` from catalog `inject` / `inject_preset` |

### Vendor OAuth — integrator escape hatch (not daigate core)

**daigate core ships generic OAuth2 only.** Non-RFC vendor flows (Anthropic Messages OAuth, OpenAI Codex, etc.) are **not** bundled in the stock CLI.

Integrators that need them register a vendor `oauth.Module` at **build time** in their own binary (same library-embed pattern as extra adapters):

```go
type Module interface {
    ProviderID() string
    LoginURL(state string) (url string, err error)
    Exchange(ctx context.Context, code string) (Material, error)
    Refresh(ctx context.Context, refreshToken string) (Material, error)
}

// integrator main.go — not stock daigate
oauth.Register(anthropic.Module{…})
```

| Example vendor | Why generic is insufficient |
|----------------|----------------------------|
| Anthropic | Custom authorize/token shape, Messages-specific inject |
| OpenAI Codex | Vendor-specific OAuth endpoints and token response |

Integrators with non-RFC vendors register `oauth.Module` at **build time** in their own binary — not in stock `daigate`.

### Refresh ownership (hard rule)

**Single writer** for OAuth refresh — only one process may refresh grants and write the store:

| Role | May refresh? |
|------|----------------|
| `credential/admin` background loop | Yes (one process) |
| `forward.Engine` on stale token | Delegates to admin RPC or inline refresh **only if** configured as sole refresher |
| Data plane replicas | **Read-only** store snapshot |

Run exactly one admin process that refreshes OAuth grants.

### OAuth vs API key profiles

In store schema, a profile has a **kind**:

```yaml
# logical profile id → storage row
my-oauth:         { kind: oauth, oauth: { authorize_url: …, token_url: …, scopes: […] } }
openai:           { kind: api_key }
acme:             { kind: api_key, inject_preset: x-api-key }
# vendor-oauth:   { kind: oauth, module: vendor }   # integrator binary only — not stock CLI
```

CLI:

```bash
daigate credential login my-oauth       # generic OAuth2
daigate credential import openai --api-key sk-…
daigate credential list               # metadata only — [security.md](security.md#credential-listing-operator)
daigate credential show 3
```

---

## Summary table

| Layer | Pluggable | Examples |
|-------|-----------|----------|
| Client → daigate (verify) | `ingress.Authenticator` | **`keyring`** (hash lookup, all keys) |
| Gateway key kinds | `static` / `issued` metadata | same verify path |
| Gateway key issue | admin API + optional issuer plugins | admin, **provision**; self-service issuers via linked plugins |
| Control-plane roles | admin vs provision tokens | provision cannot list upstream credentials |
| Secret storage | `credential.Store` + **encryption at rest** | sqlite (encrypted), memory (tests); alternate backends via linked plugins |
| Upstream headers | `inject.ApplyRoute` + catalog `inject` / `inject_preset` | multi-header map, bearer, x-api-key, custom header name |
| OAuth | `oauth/generic` from `credential_profiles` | stock CLI |
| OAuth (vendor) | `oauth.Module` at integrator build | non-RFC vendors only |
| Credential list | `GET /v1/credentials` + CLI | metadata only — [security.md § Credential listing](security.md#credential-listing-operator) |
| Admin surface | HTTP on optional `:9421` | routes — [security.md § Admin API](security.md#admin-api) |

API key issuance and OAuth are both plugin slots — store driver chooses persistence; oauth module chooses vendor flow; issuer chooses how **your** users get gateway keys. All are **Go compile-time** registration; no runtime WASM plugins.