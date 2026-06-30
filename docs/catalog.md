# Catalog and ingress wires

Operator config: **`providers.yaml`** (upstream connections) + **`models`** (catalog ids clients use in JSON `model`).

**Runtime source of truth is always local YAML** on the gateway host. Optional remote feeds (e.g. [Charm catwalk](https://catwalk.charm.land/v2/providers)) are for **metadata bootstrap only** — see [Catalog sources](#catalog-sources).

---

## Providers (`providers`)

A **provider** is one logical vendor / credential account. It can expose **multiple surfaces** (chat, images, embed, …) — each surface is one `(protocol | adapter, base_url)` pair.

**Design goal:** one `provider_ref` with OAuth once — multiple surfaces under it, not duplicate top-level provider keys per modality.

### Multi-surface (recommended)

```yaml
providers:
  openai:
    credential_profile: openai
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.openai.com
      embed:
        protocol: openai-embeddings
        base_url: https://api.openai.com
      images:
        protocol: openai-images
        base_url: https://api.openai.com
```

| Field | Level | Meaning |
|-------|-------|---------|
| `credential_profile` | **Provider** | Vault key — one OAuth grant per provider |
| `surfaces.<id>.protocol` | Surface | Passthrough handler (OpenAI/Anthropic wires) |
| `surfaces.<id>.adapter` | Surface | Translate handler name (integrator code, e.g. `myvendor`) |
| `surfaces.<id>.base_url` | Surface | Upstream root for this surface |
| `surface` on pool entry | Model pool | Which surface id to use (required for translate adapters) |

**Resolve:** ingress wire → pick surface on `provider_ref` (explicit `surface:` or auto-match passthrough protocol). Translate surfaces require explicit `surface:` in the model pool.

```yaml
models:
  gpt-5.4-mini:
    modalities:
      chat:
        wire: openai-chat-completions
        providers:
          - provider_ref: openai
            surface: chat
            model: gpt-5.4-mini
      embed:
        wire: openai-embeddings
        providers:
          - provider_ref: openai
            surface: embed
            model: text-embedding-3-small
```

If only one surface on the provider matches the wire’s protocol, `surface:` may be omitted.

### Flat entry (legacy / simple)

Single-surface vendors stay flat — equivalent to one surface:

```yaml
providers:
  anthropic:
    protocol: anthropic-messages
    base_url: https://api.anthropic.com
    credential_profile: anthropic
```

Loader normalizes to `surfaces.default` internally.

### Legacy flat `provider_ref` (integrators)

Some deployments use **one catalog row per wire** (`openai-chat`, `openai-embed`, …). daigate prefers **`providers.<vendor>.surfaces.*`** with one shared `credential_profile`. Integrators migrating flat refs normalize to `(provider_ref, surface)` in their shim — not daigate core.

| Field | Meaning |
|-------|---------|
| `protocol` | Passthrough handler id — must be registered (e.g. `openai-chat-completions`) |
| `adapter` | Translate handler name — must be registered in your binary (e.g. `myvendor`) |
| `base_url` | Upstream API root (HTTPS). Passthrough appends wire paths (`/v1/chat/completions`, …); duplicate `/v1` segments are collapsed when `base_url` already ends with `/v1` |
| `credential_profile` | Credential store key for `Store.Get` |
| `inject_preset` | How to apply auth on outbound requests |

### `provider_ref` — operator alias (rename freely)

`provider_ref` is the **key under `providers:`** in your yaml. It is **not** a vendor-global id from catwalk — it is **how the router wires models to a connection**.

```yaml
providers:
  openai-prod:
    credential_profile: team-openai
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.openai.com

  openai-dev:
    credential_profile: dev-openai
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.openai.com

models:
  gpt-5.4-mini:
    modalities:
      chat:
        providers:
          - provider_ref: openai-prod
            surface: chat
            model: gpt-5.4-mini
```

**Why keep `provider_ref`?** It names the **logical provider** (vendor + credential account). **`surface`** picks which protocol/base_url on that provider. You need two provider refs when credentials differ (`openai-prod` vs `openai-dev`), not when only the modality differs.

**Renaming**

| What | Rename? | Notes |
|------|---------|-------|
| `provider_ref` (yaml key) | **Yes** | Update every `provider_ref:` in `models` pools |
| `credential_profile` | **Yes** | Rename in yaml **and** vault (`daigate credential …`) or re-login OAuth |
| Client JSON `model` | **Your catalog id** | Independent (`gpt-5.4-mini`, `my-fast-chat`) |

**OAuth:** OAuth grant is stored under `credential_profile` in `broker.db`, not under `provider_ref`. You can rename `provider_ref` from `openai` → `openai-work` while keeping `credential_profile: team-openai` unchanged. Renaming the **profile** requires vault migration or `credential login` again.

---

## Models (`models`)

Global names clients use in JSON `model` field. Each catalog entry has one or more **modalities**.

```yaml
models:
  claude-sonnet-4-6:
    modalities:
      chat:
        wire: anthropic-messages
        providers:
          - provider_ref: anthropic-chat
            model: claude-sonnet-4-6
        strategy: failover

  text-embedding-3-small:
    modalities:
      embed:
        wire: openai-embeddings
        providers:
          - provider_ref: openai
            surface: embed
            model: text-embedding-3-small

  gpt-image-2:
    modalities:
      image:
        wire: openai-images-generations
        providers:
          - provider_ref: openai
            surface: images
            model: gpt-image-2

router:
  modality_defaults:
    chat: anthropic-messages
    embed: openai-embeddings
    image: openai-images-generations
```

### Modalities

daigate stores modalities as **`models.<id>.modalities.<key>`** — the `<key>` string is **operator-defined** in `providers.yaml`. There is no fixed enum in core; any yaml key works if resolve can pick a unique row for the wire.

**Wire-implied keys** (daigate sets a default hint from ingress path):

| Key | Ingress path | Typical use |
|-----|--------------|-------------|
| `embed` | **`/v1/embeddings`** | Embeddings |

**Common operator keys** (documented convention; not hardcoded in resolve except where noted):

| Key | Ingress path | Typical use |
|-----|--------------|-------------|
| `chat` | `/v1/chat/completions`, `/v1/messages`, `/v1/responses` | LLM chat |
| `image` | `/v1/images/generations`, `/edits` — or same chat wire for vision | Image gen/edit or vision on chat |
| `speech` | `/v1/audio/speech` | TTS |
| `video` | `/v1/videos/generations`, `GET /v1/videos/{id}` | Video gen |

Hosts may add more keys (`search_web`, `audio_in`, …) for extra surfaces on the same wire. Those names live in **your** catalog only; map them to **`X-Catalog-Modality`** at the host edge. Do not assume daigate knows product-specific labels.

**Disambiguation default:** when the only ambiguity is `chat` plus keys named `search_web` / `search_x`, resolve defaults to `chat` without a header. All other multi-modality wires require **`X-Catalog-Modality`** (e.g. `chat` + `image` on `openai-chat-completions`).

### Resolution rules

1. **Ingress path** picks **wire** and wire-implied modality hint when applicable (`/v1/embeddings` → `embed` + `openai-embeddings`).
2. Request `model` loads catalog entry.
3. Modality selects `modalities.<name>` (e.g. `modalities.embed`). When one model + wire maps to **multiple** modalities (e.g. `chat` and `image` on `openai-chat-completions`), set ingress header **`X-Catalog-Modality`** to the yaml key. daigate strips it before upstream relay.
4. **Strategy** picks provider from pool (see below).
5. `provider_ref` → provider surface (`protocol` or `adapter`, `base_url`, `credential_profile`).
6. Pool `model` field → upstream provider model string.

### Pool strategies

| Strategy | Default | Behavior |
|----------|---------|----------|
| `failover` | yes | Tries providers in yaml order. On connection errors or HTTP 429/502/503/504, advances to the next pool member. |
| `round_robin` | no | Picks one provider per request in rotation. No automatic fallback to siblings on failure. |

`sticky` is not supported. Existing configs must use `failover` or `round_robin`.

---

## Ingress wires

| Wire ID | HTTP | Client expectation |
|---------|------|-------------------|
| `anthropic-messages` | `POST /v1/messages` | Anthropic SDK |
| `openai-chat-completions` | `POST /v1/chat/completions` | OpenAI SDK / compat |
| `openai-responses` | `POST /v1/responses` | **OpenAI Responses API** ([reference](https://developers.openai.com/api/reference/responses/overview)) — any compatible upstream |
| **`openai-embeddings`** | **`POST /v1/embeddings`** | **`client.embeddings.create`** |
| `openai-images-generations` | `POST /v1/images/generations`, `/edits` | OpenAI Images API |
| `openai-audio-speech` | `POST /v1/audio/speech` | OpenAI TTS |
| `openai-videos` | `POST /v1/videos/generations`, `GET /v1/videos/{id}` | OpenAI-compatible video |

Also: `GET /v1/models`, `GET /v1/healthz`.

### Wire rules

**Chat:** each modality declares an ingress `wire` (`openai-chat-completions` or `anthropic-messages`). Catalog picks a provider `surface` whose `protocol` matches that wire (or an explicit pool `surface:`); the core **passthrough** handler relays the request unchanged. Pool `model:` rewrites the upstream model id (catalog alias → vendor string). Vendor-specific chat APIs use `surface.adapter` (operator-linked translate handler).

**Embeddings:** OpenAI-compatible ingress body. Passthrough `openai-embeddings` relays to upstream `/v1/embeddings`.

**Media:** native paths only; `surface.adapter` for translate, `surface.protocol` for passthrough ([media.md](media.md)).

---

## Passthrough protocols (core registry)

Core auto-matches these protocols to ingress wires when no explicit `surface:` is set:

| `protocol` | Modality | Role |
|------------|----------|------|
| `openai-chat-completions` | chat | Passthrough `/v1/chat/completions` |
| `anthropic-messages` | chat | Passthrough `/v1/messages` |
| `openai-responses` | chat | Passthrough `POST /v1/responses` |
| `openai-embeddings` | embed | Passthrough `/v1/embeddings` |
| `openai-images` | image | Passthrough `/v1/images/*` |
| `openai-tts` | speech | Passthrough `/v1/audio/speech` |
| `openai-videos` | video | Passthrough `/v1/videos/*` |

## Translate adapters

Core ships **no** translate adapters. Operator binaries link optional handlers via `adaptersdk`. Operators set `adapter:` on surfaces — not vendor protocol names:

| `adapter` | Modality | Package |
|-----------|----------|---------|
| `myvendor` | image, speech, … | Operator Go module via `adaptersdk` ([adapters.md](adapters.md)) |

Customers use catalog `model` on native paths only.

---

## Catalog sources

Three layers — do not conflate them.

| Layer | What | Where |
|-------|------|--------|
| **Adapters** | Executable handlers (passthrough protocols + translate adapters) | Compiled into binary; discover via `daigate adapters list` |
| **Model metadata** | Upstream model ids, pricing hints, context window, capabilities | Optional remote JSON (catwalk-style) |
| **Operator catalog** | Enabled models, aliases, pools, `base_url`, `credential_profile` | **`providers.yaml` on disk** |

### Manual (default, v1)

Operator edits `providers.yaml` directly. Adapter packages document wiring hints in Go comments; there is no preset or manifest scaffolding.

Best when you have a small fixed set of models (typical self-hosted gateway).

### Why not remote-only catalogs?

| Concern | Local `providers.yaml` | Remote-only (catwalk) |
|---------|------------------------|------------------------|
| Secrets / OAuth profiles | Yes | No |
| Custom catalog aliases (`gpt-5.4-mini`) | Yes | No |
| Failover / round_robin pools | Yes | No |
| Private upstream `base_url` | Yes | No |
| Adapter / protocol binding | Yes | No |
| Air-gapped deploy | Yes | Problematic |

**Recommendation:** treat external model lists as **reference docs** — commit operator intent to `providers.yaml`. Routing always stays local yaml.

