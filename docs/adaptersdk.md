# Adapter SDK (`adaptersdk`)

Third parties building on daigate import **`adaptersdk`** — not `gateway` internals.

```go
import (
    "github.com/subosito/daigate/adaptersdk"
    "github.com/subosito/daigate/adaptersdk/handler"
)
```

| Role | Import |
|------|--------|
| **Adapter author** (Git module) | `adaptersdk` |
| **Gateway embedder** | `gateway` + explicit `[]adaptersdk.Adapter` |

---

## `Adapter` interface

```go
type Adapter interface {
    Name() string
    Register(reg *Registry) error
}
```

```go
func New() adaptersdk.Adapter { return &Adapter{} }
```

`Name()` is the operator-facing id — it must match `surface.adapter` in `providers.yaml` and `adapters.enable` in `daigate.yaml`.

---

## Registry

Two dispatch paths:

| Registration | Map | Key |
|--------------|-----|-----|
| `RegisterChat`, `RegisterImage`, … | `ChatHandlers`, `ImageHandlers`, … | protocol name |
| `RegisterImageAdapter`, `RegisterSpeechAdapter`, … | `ImageAdapters`, `SpeechAdapters`, … | adapter name |

Passthrough relay handlers use protocol maps. Translate handlers use adapter maps — operators set `surface.adapter` in yaml, not vendor protocol strings.

Discovery: `daigate adapters list` prints registered protocols and adapter names from the compiled binary.

---

## Handler interfaces

Defined in `adaptersdk/handler` — native ingress shapes (OpenAI). Adapters implement; core `wire` dispatches.

| Interface | Methods |
|-----------|---------|
| `Chat` | `Protocol()`, `Forward(ctx, client, Target, body, hdr)` |
| `Embed` | same |
| `Image` | `Forward(..., ingressPath, body, hdr)` |
| `Speech` | chat-shaped `Forward` |
| `Video` | image-shaped `Forward` |

`handler.Target` carries `catalog.Target` + `store.Material` (credential inject).

---

## Register example

Translate image adapter:

```go
func (a *Adapter) Register(reg *adaptersdk.Registry) error {
    adaptersdk.RegisterImageAdapter(reg, a.Name(), &ImageHandler{})
    return nil
}
```

Passthrough surfaces in the same adapter:

```go
adaptersdk.RegisterChat(reg, &passthrough.ChatHandler{
    ProtocolName: "openai-chat-completions",
    WireID:       catalog.WireOpenAIChat,
})
```

Document operator `providers.yaml` wiring in a comment on the adapter struct (see any translate adapter in an integrator repo).

---

## Operator catalog

Adapters do **not** ship `manifest.yaml` or preset scaffolding. Operators write full `providers.yaml`:

- `providers.<name>.surfaces.*` with `protocol` or `adapter` + `base_url`
- `models.*.modalities.*` with `provider_ref`, `surface`, upstream `model`

See [catalog.md](catalog.md) for surface and pool examples.

---

## Auth

Adapters never touch `broker.db` or raw API keys. `credential/inject.Apply` runs in handlers using `handler.Target.Material`.

OAuth profiles are operator config in `daigate.yaml` (`credential_profiles`) — not embedded in adapter packages. See [auth.md](auth.md).

---

## Module layout

```text
daigate-adapter-corp/
  go.mod              # require github.com/subosito/daigate
  adapter.go          # New(); Register(); wiring comments
  images.go           # handler.Image
  adapter_test.go
```

Naming: `github.com/<org>/daigate-adapter-<name>`.

---

## Stability

`adaptersdk` is the public, semver-stable surface for adapter authors. Gateway internals (`wire/`, `gateway/`) may change without breaking adapters that only import `adaptersdk`.