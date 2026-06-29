# Media endpoints (image, speech, video)

## Product rule

**Customers see one predictable API — native OpenAI paths.**  
**How daigate talks to upstream vendors is internal** — passthrough relay or a linked translate adapter.

```text
Customer                          daigate                           Upstream
────────                          ─────────                           ────────
POST /v1/images/generations   →   catalog + passthrough/adapter   →   OpenAI-compatible upstream
POST /v1/audio/speech         →   catalog + passthrough/adapter   →   OpenAI-compatible upstream
POST /v1/videos/generations   →   catalog + passthrough/adapter   →   OpenAI-compatible upstream
```

The customer picks a **catalog model id** and uses the **OpenAI SDK** with `OPENAI_BASE_URL` pointing at daigate. They never choose a provider driver or learn vendor URL shapes.

---

## Public ingress (only surface)

| Modality | Paths | OpenAI SDK |
|----------|-------|------------|
| **Image** | `POST /v1/images/generations`, `POST /v1/images/edits` | `images.generate`, `images.edit` |
| **Speech** | `POST /v1/audio/speech` | `audio.speech.create` |
| **Video** | `POST /v1/videos/generations`, `GET /v1/videos/{request_id}` | OpenAI-compatible submit + poll |

```bash
export OPENAI_BASE_URL="http://127.0.0.1:9420/v1"
export OPENAI_API_KEY="sk-dg-…"
```

Same base URL and gateway key as chat and embeddings ([ingress.md](ingress.md)).

**Note:** `POST /v1/embeddings` is not media — see [catalog.md](catalog.md). Passthrough embed uses `protocol: openai-embeddings`.

`/v1/responses` is **chat only** — not a media ingress.

---

## Passthrough vs translate

| Kind | Operator yaml | When |
|------|---------------|------|
| **Passthrough** | `protocol: openai-images` (or `openai-tts`, `openai-videos`) | Upstream already speaks OpenAI wire |
| **Translate** | `adapter: myvendor` | Ingress OpenAI shape → vendor API (integrator Go package) |

**Stock `daigate` ships passthrough only.** Translate adapters are **operator code** — link your own `adaptersdk` package in the operator binary ([adapters.md](adapters.md)).

Passthrough-only image/speech/video need **no Go package** — set `protocol: openai-images` / `openai-tts` / `openai-videos` on the surface.

---

## Request lifecycle

1. **Ingress auth** — gateway API key (same as chat).
2. **Wire select** — path → modality.
3. **Catalog resolve** — `model` → provider pool entry + upstream model string.
4. **Handler select** — `target.Adapter` → translate adapter map; else `target.Protocol` → passthrough map.
5. **Credential inject** — `Store.Get(credential_profile)`; strip client secrets.
6. **Execute** — translate if needed; normalize response to ingress wire.

```text
Client  POST /v1/images/generations
        { "model": "gpt-image-2", "prompt": "a cat" }
           │
           ▼
daigate  catalog → surface.protocol: openai-images
           passthrough → POST …/v1/images/generations
           │
           ▼
Client  ← OpenAI images shape
```

---

## Per-modality notes

### Image

Native request/response follows OpenAI Images API. Passthrough relays body and response when upstream is OpenAI-compatible. A translate adapter may map ingress JSON to a vendor-native API and back.

### Speech

Native body: `model`, `input`, `voice`, `response_format`. Passthrough relays when upstream speaks OpenAI TTS. Translate adapters map fields when the vendor wire differs.

### Video

Native async contract: `POST /v1/videos/generations` then `GET /v1/videos/{id}`. Passthrough when upstream is OpenAI-compatible; translate adapters may own poll semantics.

---

## Catalog wiring (passthrough)

```yaml
providers:
  openai:
    credential_profile: openai
    surfaces:
      images:
        protocol: openai-images
        base_url: https://api.openai.com
      speech:
        protocol: openai-tts
        base_url: https://api.openai.com
      video:
        protocol: openai-videos
        base_url: https://api.openai.com

models:
  gpt-image-2:
    modalities:
      image:
        wire: openai-images-generations
        providers:
          - provider_ref: openai
            surface: images
            model: gpt-image-2
```

| Field | Meaning |
|-------|---------|
| `wire` | Customer HTTP path(s) for this model |
| `surface.adapter` | Translate handler name (integrator binary only) |
| `surface.protocol` | Passthrough handler for OpenAI/Anthropic wires |
| `model` on pool entry | Upstream provider model string |

Translate surfaces require explicit `surface:` in the model pool and a linked adapter — see [adapters.md](adapters.md).

---

## Layering

```text
CORE — customer-visible
  /v1/images/generations, /v1/images/edits
  /v1/audio/speech
  /v1/videos/generations, /v1/videos/{id}

INTERNAL — adapters (adaptersdk)
  passthrough/           # openai-images, openai-tts, openai-videos (shipped)
  your-repo/myvendor/    # translate (integrator Go package)
```

Third parties ship an **adaptersdk** adapter — see [adaptersdk.md](adaptersdk.md).

---

## Decision summary

1. **One customer API** — native `/v1/images/*`, `/v1/audio/speech`, `/v1/videos/*` only.
2. **Passthrough media** — default path; `protocol: openai-*` on surface.
3. **Translate adapters** — Go packages only when shape conversion is required; not in stock CLI.
4. **Same auth** — gateway API key on all paths.