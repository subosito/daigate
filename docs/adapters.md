# Composable adapters

## Goal

**Core is vendor-agnostic and thin.** Vendor logic lives in **adapters** — pure Go packages that register handlers at compile time.

| Layer | Package | Audience |
|-------|---------|----------|
| Gateway core | `gateway`, `wire`, `catalog` | Embedders |
| **Adapter SDK** | **`adaptersdk`** | Third-party adapter authors ([adaptersdk.md](adaptersdk.md)) |
| Core adapter | `passthrough` | Protocol relay (chat, embed, image, speech, video) |
| Translate adapters | Integrator / third-party Go modules | Shape conversion when upstream wire differs |
| CLI compose | `compose/` | Core CLI — passthrough only |

```text
┌─────────────────────────────────────────────────────────────┐
│ CORE                                                         │
│  gateway · wire · catalog · upstream · credential            │
└───────────────────────────┬─────────────────────────────────┘
                            │ []adaptersdk.Adapter
        ┌───────────────────┴───────────────────┐
        ▼                                       ▼
  passthrough                           operator/myvendor
  (core relay)                          (translate shims)
```

Customers see **OpenAI-compatible ingress only** — chat, `/v1/embeddings`, media ([ingress.md](ingress.md), [media.md](media.md)). Operators configure `adapter:` on surfaces for translate paths; passthrough vendors use `protocol:` only.

---

## Passthrough vs translate

| Kind | Operator yaml | Registry key | When |
|------|---------------|--------------|------|
| **Passthrough** | `protocol: openai-chat-completions` | protocol name | Upstream speaks OpenAI/Anthropic wire |
| **Translate** | `adapter: myvendor` | adapter name | Ingress OpenAI shape → vendor API |

Passthrough-only upstreams need **no Go package** — `providers.yaml` + `passthrough` adapter enabled is enough.

Go packages are only required when daigate must **translate** request/response shapes between ingress and upstream.

---

## What core does (and does not)

| Core owns | Core does **not** own |
|-----------|------------------------|
| HTTP server, ingress auth, wire routing | Vendor JSON shapes |
| Catalog resolve (`surface.adapter` or `surface.protocol`) | Adapter authoring helpers |
| Dispatch to `adaptersdk` registry | Any import of vendor `adapters/*` |
| Credential inject + upstream relay | |

Core never `switch provider { case "openai": … }`.

---

## `passthrough/` — core relay

Top-level package (not under `adapters/`):

```text
passthrough/
  adapter.go    # New(); Register(adaptersdk.Registry)
  media.go
```

Registers protocol-keyed handlers:

| Protocol | Modality |
|----------|----------|
| `openai-chat-completions` | chat |
| `anthropic-messages` | chat |
| `openai-responses` | chat |
| `openai-embeddings` | embed |
| `openai-images` | image |
| `openai-tts` | speech |
| `openai-videos` | video |

---

## Translate adapters — integrator code

**Not shipped** in stock CLI. Implement in your operator binary:

```text
your-repo/adapters/
  myvendor/    # translate image/speech/… (your Go package)
```

Each adapter imports **`adaptersdk`** and exports:

```go
func New() adaptersdk.Adapter
```

Translate handlers register with `RegisterImageAdapter`, `RegisterSpeechAdapter`, etc. Passthrough surfaces inside a translate adapter (e.g. chat on the same vendor) still use `RegisterChat` / `RegisterEmbed`.

Wiring hints live as Go comments on the adapter struct — operators write full `providers.yaml` by hand ([catalog.md](catalog.md)).

**Dependency rule:** `gateway/` imports `adaptersdk` only — never vendor `adapters/*`.  
Operator binaries pass adapters explicitly to `compose.FromConfig(enable, available)` or `gateway.New`.

---

## Compose tooling

```text
compose/
  compose.go    # DefaultAdapters(), FromConfig(enable, available)
```

Stock CLI:

```go
reg, _ := compose.FromConfig(cfg.Adapters.Enable, compose.DefaultAdapters())
```

Integrator binary:

```go
available := []adaptersdk.Adapter{passthrough.New(), myvendor.New()}
reg, _ := compose.FromConfig(cfg.Adapters.Enable, available)
```

---

## Config

```yaml
adapters:
  enable: [passthrough]              # stock CLI
  # enable: [passthrough, myvendor]  # integrator binary — names must match linked adapters
```

CLI:

```bash
daigate adapters list
daigate adapters doctor
```

`adapters list` introspects the compiled registry (protocols + translate adapter names).  
`adapters doctor` checks every catalog `protocol` / `adapter` has a registered handler.

---

## Operator `providers.yaml`

Passthrough chat:

```yaml
providers:
  openai:
    credential_profile: openai
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.openai.com
```

Translate image (requires linked `myvendor` adapter in binary):

```yaml
providers:
  acme:
    credential_profile: acme
    surfaces:
      image:
        adapter: myvendor
        base_url: https://api.acme.example/v1
      chat:
        protocol: openai-chat-completions
        base_url: https://api.acme.example/compatible-mode/v1

models:
  acme-image-v1:
    modalities:
      image:
        wire: openai-images-generations
        providers:
          - provider_ref: acme
            surface: image
            model: acme-image-v1
```

Wire dispatch: `target.Adapter` → `ImageAdapters[name]`; else `target.Protocol` → `ImageHandlers[protocol]`.

---

## Adding a new adapter

1. New Go package in your module (or publish `github.com/acme/daigate-adapter-foo`)
2. Implement `adaptersdk.Adapter` + handler interfaces ([adaptersdk.md](adaptersdk.md))
3. `Register()` with `RegisterImageAdapter(reg, a.Name(), …)` (or speech/embed/video/chat)
4. Document operator wiring in Go comments on the adapter struct
5. Link in operator binary; add name to `adapters.enable`
6. Write `providers.yaml` surfaces and model pools by hand

See also: [adaptersdk.md](adaptersdk.md), [runtime.md](runtime.md), [architecture.md](architecture.md).