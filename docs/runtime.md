# Runtime — CLI, library, and external adapters

## Summary

| Surface | Who | Purpose |
|---------|-----|---------|
| **CLI** (`daigate`) | Operators | Run gateway, manage credentials, list adapters — **primary product** |
| **Library** (`gateway`) | Integrators | Embed gateway in another Go binary — **optional, same engine as CLI** |
| **adaptersdk** | Adapter authors | Publish translate adapters in operator or third-party modules |

**Recommendation:** Ship **CLI as the default product**. Build a **custom operator binary** when you need extra credential backends, self-service issuers, or translate adapters.

End users of LLM APIs never import Go — they point `OPENAI_BASE_URL` at the data plane (`:9420` by default).

---

## Listeners

| Plane | Default | Config key |
|-------|---------|------------|
| Data (SDK / curl) | `:9420` | `serve.data_listen` |
| Admin (credentials, keys, OAuth) | `127.0.0.1:9421` | `admin.listen` |

Set `admin.enable: false` in embed-only binaries that supply credentials via `Store` and skip the admin HTTP surface.

---

## Adapter loading (decision: compile-time Go only)

| Model | How | WASM? |
|-------|-----|-------|
| Passthrough upstreams | `providers.yaml` + `adapters.enable: [passthrough]` | No |
| Translate adapters (vendor-specific image/speech/…) | Go module + `adaptersdk` in **your binary** | No |
| Credential backends / issuers | Go module + `RegisterCredentialBackend` / `RegisterAdminIssuer` in **your binary** | No |
| Untrusted third-party adapter | **Not supported** — WASM out of scope | — |

`adapters.enable` **filters** adapters compiled into the binary. It does not download code.

```text
Build time:  compose.FromConfig(enable, availableAdapters[])
Run time:    adapters.enable picks subset
```

---

## How the CLI works

```text
daigate serve
  │
  ├─ read daigate.yaml (listen, paths, adapters.enable)
  ├─ read providers.yaml (catalog)
  ├─ open broker.db (encrypted sqlite store)
  ├─ compose.FromConfig() → adaptersdk.Registry   # passthrough only in stock CLI
  ├─ gateway.New(Config{Adapters, Catalog, Store, …})
  ├─ observability.Boot("daigate")              # OTLP when OTEL_* set
  └─ ListenAndServe → :9420 (data) + :9421 (admin, optional)
       defer observability.ShutdownGraceful()
```

### Operator files

**`daigate.yaml`** — process config:

```yaml
serve:
  data_listen: "127.0.0.1:9420"
  catalog: providers.yaml

adapters:
  enable: [passthrough]
```

**`providers.yaml`** — models + upstream providers ([catalog.md](catalog.md)).

**`broker.db`** — encrypted sqlite credential store ([auth.md](auth.md)). Requires `DAIGATE_BROKER_KEY` (or `credential.encryption.key_file`).

**Plugin slots** — core exposes `credential.backend` + opaque `backend_config`, and `ingress.issuers[]` with opaque `config`. Linked operator modules decode yaml and register via `gateway.RegisterCredentialBackend` / `gateway.RegisterAdminIssuer`.

### CLI commands

| Command | Role |
|---------|------|
| `daigate serve` | Start gateway |
| `daigate credential list\|show` | Credential metadata (no secrets) |
| `daigate credential login\|import` | Upstream credentials / OAuth |
| `daigate keys create [--static]` | Issue gateway keys |
| `daigate admin token create --role provision` | CI provision token |
| `daigate adapters list` | Registered protocols and translate adapters |
| `daigate adapters doctor` | Catalog routes ⊆ registered handlers |

---

## Custom operator binary

Same engine as CLI; you supply adapters, backends, and issuers at link time:

```go
import (
    "github.com/subosito/daigate/gateway"
    corecompose "github.com/subosito/daigate/compose"
    "github.com/subosito/daigate/passthrough"
    myvendor "example.com/my/adapters/myvendor"
)

available := []adaptersdk.Adapter{passthrough.New(), myvendor.New()}
reg, _ := corecompose.FromConfig(cfg.Adapters.Enable, available)
gw, _ := gateway.New(gateway.Config{Adapters: reg, CatalogPath: "providers.yaml", Store: store})
```

Blank-import backend and issuer plugins in `main` or a `register` package as needed.

---

## How the library works

```go
import (
    "github.com/subosito/daigate/gateway"
    "github.com/subosito/daigate/compose"
)

reg, _ := compose.FromConfig([]string{"passthrough"}, compose.DefaultAdapters())
gw, err := gateway.New(gateway.Config{
    Adapters:    reg,
    CatalogPath: "providers.yaml",
    Store:       store,
    AdminAuth:   authenticator,
})

// Standalone custom binary — same as CLI:
observability.Boot("daigate")
defer observability.ShutdownGraceful()
_ = gw.ListenAndServe(context.Background())
```

**When to use the library**

- Embed gateway inside an existing Go service.
- Unit/integration tests with memory store.
- Custom binary linking translate adapters + vendor `oauth.Module` not in stock CLI.

**Default path:** `daigate serve` + YAML, or a custom operator binary that calls `gateway.Serve`.

### Observability when embedding in another service

If the host process already exports OTel, **do not** call daigate `Boot`. Call `Hook` after host init:

```go
hostobs.Boot("my-service")       // host package — owns OTLP + slog bridge
defer hostobs.ShutdownGraceful()

daigateobs.Hook("daigate")   // binds spans/metrics to global providers

gw, _ := gateway.New(...)
gw.ListenAndServe(ctx)           // never Boot or Shutdown OTel here
```

→ [observability.md](observability.md)

---

## Configuration ↔ adapters ↔ catalog

```text
daigate.yaml              providers.yaml                    binary
──────────────              ──────────────                    ──────
adapters.enable: [myvendor] providers.foo.surfaces.*        your/adapters/myvendor
                             models.*.modalities.*
                             .surface / provider_ref         passthrough protocols or adapter names
```

Startup validation:

1. Every catalog passthrough `protocol` must be in a protocol handler map.
2. Every catalog translate `adapter` must be in an adapter handler map.
3. `daigate adapters doctor` reports missing handlers.

---

## Decision table

| Question | Answer |
|----------|--------|
| CLI only? | **CLI is the product** for operators. |
| Drop library? | **No** — CLI wraps `gateway.New`. |
| External adapters? | **Go compile-time link** — core passthrough or your modules. |
| WASM? | **Out of scope** — skip; use your own Go adapter. |
| Runtime adapter download? | **No** |
| `.so` plugins? | **No** |

See also: [adapters.md](adapters.md), [adaptersdk.md](adaptersdk.md), [architecture.md](architecture.md).