# Security

Threat model, admin route access, and credential-listing rules. [auth.md](auth.md) covers ingress/egress interfaces, OAuth flows, and encryption setup.

---

## Threat model

| In scope | Out of scope |
|----------|--------------|
| Stolen `broker.db` / backups | Compromised host with broker encryption key in env |
| Leaked admin or ingress tokens via logs/CLI | HSM / cloud KMS |
| Casual operator mistakes (`--json` piping secrets) | Nation-state |
| | Multi-tenant SaaS isolation |

---

## Planes and trust boundaries

```text
  Internet / SDK          Loopback / private net          Upstream APIs
        │                          │                            │
        ▼                          ▼                            ▼
  :9420 data plane            :9421 admin plane            HTTPS vendors
  ingress keys only           admin auth required          injected upstream auth
  never sees broker.db        credential CRUD + list       never sees ingress keys
```

| Plane | Bind default | Auth | Sees secrets? |
|-------|--------------|------|---------------|
| Data `:9420` | configurable | Gateway API key (`ingress`) | **No** — `Store.Get` inside forward path only |
| Admin `:9421` | `127.0.0.1` | Admin or provision token (separate from gateway keys) | **List: metadata only** — never api_key / tokens over HTTP |

**Hard rule:** upstream secrets exist in process memory during `Store.Get` → `inject` → forward. They are **never** returned on admin HTTP responses or CLI list output.

---

## Encryption at rest

See [auth.md § Encryption](auth.md#encryption-at-rest-required).

| Check | Requirement |
|-------|-------------|
| `credentials.data` on disk | AES-256-GCM ciphertext envelope only |
| Master key | `DAIGATE_BROKER_KEY` or `key_file`; chmod 0600; not in git (`DAIGATE_VAULT_KEY` still accepted as legacy alias) |
| Missing key at startup | Fail closed |
| Gateway ingress keys | Argon2id hash in DB; plaintext shown once at `keys create` |
| Logs / metrics | Never log decrypted material, admin tokens, or ingress secrets |

---

## Credential listing (operator)

Operator-facing list — metadata only. Replaces older vault patterns that returned inject material over HTTP snapshot.

### CLI

```bash
daigate credential list              # table
daigate credential list --json       # same fields — still no secrets
daigate credential show <id>         # one row, metadata only
```

**Table columns (shipped):** `id`, `profile` (provider), `kind` (`oauth` | `api_key`), `status`, `identityKey` (email/account if oauth), `createdAt`, `updatedAt`, `source` (when Vault-backed).

**Never expose:** `--show-secrets`, `--dump-key`, or JSON fields for `access`, `refresh`, `key`.

`CredentialSummary` (stable JSON):

```json
{
  "id": 3,
  "profile": "openai",
  "kind": "oauth",
  "status": "active",
  "identityKey": "user@example.com",
  "createdAt": 1719000000000,
  "updatedAt": 1719000000000,
  "source": "sqlite"
}
```

### Internal forward path (not HTTP)

```go
// gateway forward — in-process only
mat, err := store.Get(ctx, profile)   // decrypt; inject; discard after request
```

Split deploy (router without admin): read-only `broker.db` + broker encryption key on disk — same as in-process `Get`, **not** admin HTTP snapshot with tokens.

Poll `GET /v1/snapshot` or subscribe to `GET /v1/snapshot/stream`; re-fetch `GET /v1/credentials` when `generation` changes.

---

## Admin authentication

Two **control-plane roles** — do not give CI the admin token.

| Role | Purpose | Vault `GET /v1/credentials` | `POST /v1/keys` |
|------|---------|----------------------------|-----------------|
| **admin** | Operators, OAuth login, import | Yes | Yes (`/v1/keys`) |
| **provision** | CI/CD smoke, ephemeral app keys | **No** | Yes (`POST /v1/keys` on admin listener only) |

| Rule | Detail |
|------|--------|
| Separate from ingress | Gateway `sk-dg-…` keys **must not** authorize admin or provision routes |
| Gateway keys in DB | Argon2id hash only; **`keyring`** verify (all keys, no env shortcut) |
| Key kinds | `static` (no expiry default) / `issued` (TTL) — same hash path |
| No env-stored gateway secrets | `static` = DB row via `keys create --static` or bootstrap |
| Default bind | `127.0.0.1:9421` |
| OAuth callback | Admin role; CSRF `state`; PKCE |
| Provisioner caps | `admin.provision.max_ttl`, `admin.provision.scopes` (fixed scopes on minted keys); `admin.keys.max_ttl` + `admin.keys.scopes` allowlist for `POST /v1/keys` |
| CI bootstrap | `POST /v1/keys` on admin listener with provision token — [ingress.md](ingress.md) |

Dev-only escape: `admin.auth: disabled` with loopback bind — **document as unsafe**, not prod default.

Provision deny paths (`/v1/credentials`, `/v1/snapshot`, and prefixes) are **hardcoded** in `ingress/adminauth`.

---

## Admin API

Bind default `127.0.0.1:9421`. Admin bearer tokens are **separate** from gateway `sk-dg-…` keys on `:9420`.

| Method | Path | Role | Notes |
|--------|------|------|-------|
| GET | `/v1/healthz` | admin or provision | Liveness |
| GET | `/v1/credentials` | **admin** | `[]CredentialSummary` — metadata only |
| GET | `/v1/credentials/:id` | **admin** | one summary |
| POST | `/v1/credentials` | **admin** | import `api_key` |
| GET | `/v1/snapshot` | **admin** | generation heartbeat `{generation, generatedAt, serverNowMs}` — no credential rows |
| GET | `/v1/snapshot/stream` | **admin** | SSE `event: snapshot` when `generation` changes |
| POST | `/v1/keys` | **admin** or **provision** | create gateway key; provision → `issued` only |
| DELETE | `/v1/keys/:id` | **admin** or **provision** | revoke gateway key |

```yaml
admin:
  listen: 127.0.0.1:9421
  tokens:
    admin_env: DAIGATE_ADMIN_TOKEN
    provision_env: DAIGATE_PROVISION_TOKEN
```

Key mint caps, provision scopes, and CI bootstrap: [auth.md](auth.md), [ingress.md § CI/CD](ingress.md#cicd--smoke-tests).

---

## Ingress authentication

| Rule | Detail |
|------|--------|
| Strip client upstream auth | Remove client `Authorization` / `x-api-key` before upstream forward |
| Inject from vault only | `inject.Applier` after `Store.Get` |
| Loopback trust | Single-node may bind `:9420` on 127.0.0.1; prod edge terminates TLS |

---

## Logging and errors

| Do | Don't |
|----|-------|
| Log `profile`, `credential_id`, `generation` | Log tokens, api keys, OAuth codes |
| Redact upstream 401 bodies in debug | Echo inbound Authorization to logs |
| `Cache-Control: no-store` on admin JSON | Cache credential responses |

---

## References

- [auth.md](auth.md) — ingress/egress interfaces, OAuth, encryption setup
- [ingress.md](ingress.md) — SDK env vars, CI key mint
- [runtime.md](runtime.md) — listeners, config