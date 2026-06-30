# 07 — Compatible ingress (not forward proxy)

## Product model

**daigate is the API server.** Clients do not point at Anthropic/OpenAI and tunnel through an HTTP forward proxy. They point SDK env vars at **daigate** and use **gateway-issued** API keys plus **catalog model names**.

```text
  WRONG (not daigate public API)          RIGHT (daigate v1)

  OPENAI_BASE_URL=https://api.openai.com   OPENAI_BASE_URL=http://daigate:9420/v1
  HTTP_PROXY=http://daigate:9401         OPENAI_API_KEY=sk-dg-…   ← gateway key
  + forward inject                         OPENAI_MODEL=gpt-5.4-mini  ← catalog id
```

This matches how developers already use **LiteLLM**, **Portkey**, and **Helicone**: change base URL + API key, keep the SDK.

---

## Listeners

| Plane | Default | Client / operator use |
|-------|---------|----------------------|
| **Data** | `:9420` (`serve.data_listen`) | `OPENAI_BASE_URL=http://host:9420/v1` — all `/v1/*` ingress below |
| **Admin** | `127.0.0.1:9421` (`admin.listen`) | `DAIGATE_ADMIN_URL` — credentials, OAuth, `POST /v1/keys` |

See [docs/README.md](README.md) for env vars and CI smoke layout.

---

## What we expose (ingress)

daigate terminates standard LLM HTTP paths on the **data plane** (`:9420` by default):

| Wire | Path | Typical client env |
|------|------|-------------------|
| OpenAI Chat Completions | `POST /v1/chat/completions` | `OPENAI_BASE_URL`, `OPENAI_API_KEY` |
| Anthropic Messages | `POST /v1/messages` | `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY` |
| OpenAI Responses | `POST /v1/responses` | OpenAI SDK or raw HTTP |
| **Embeddings** | **`POST /v1/embeddings`** | Same `OPENAI_BASE_URL` + key; `client.embeddings.create` |
| **Image** | `POST /v1/images/generations`, `/edits` | Same `OPENAI_BASE_URL` + key |
| **Speech / TTS** | `POST /v1/audio/speech` | Same `OPENAI_BASE_URL` + key |
| **Video** | `POST /v1/videos/generations`, `GET /v1/videos/{id}` | Same `OPENAI_BASE_URL` + key |
| Discovery | `GET /v1/models` | Optional |
| Health | `GET /v1/healthz` | Ops |

Media uses the **same** `OPENAI_BASE_URL` + gateway key. Upstream vendor choice is internal — passthrough relay or a linked translate adapter. See [media.md](media.md).

**Client `Authorization` / `x-api-key`** authenticates to **daigate**, not to the upstream vendor. daigate replaces those headers with **provider credentials** from the vault before the outbound call.

**Ambiguous routing:** when one catalog model shares an ingress wire across multiple `modalities.<key>` rows, set **`X-Catalog-Modality`** to the yaml key (e.g. `chat`, `image`). Key names are operator-defined — see [catalog.md](catalog.md) § Modalities.

---

## Minimal operator setup

### 1. Operator configures upstream (once)

`providers.yaml` + credential vault — real provider URLs and secrets live **only on the server**:

```yaml
providers:
  openai:
    credential_profile: openai
    surfaces:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.openai.com

models:
  gpt-5.4-mini:
    modalities:
      chat:
        wire: openai-chat-completions
        providers:
          - provider_ref: openai
            surface: chat
            model: gpt-5.4-mini
```

```bash
daigate credential import openai --api-key sk-…
# or: daigate credential login my-oauth   # generic OAuth profile in daigate.yaml
```

### 2. Operator issues a gateway key

```bash
# long-lived (solo dev / internal) — kind=static, hash in DB, no env secret
daigate keys create --static --name default

# per app / tenant — kind=issued
daigate keys create --name my-app --ttl 720h
# → sk-dg-abc123… shown once
```

### 3. Developer points their SDK at daigate

**OpenAI-compatible stack** (OpenAI SDK and compat clients):

```bash
export OPENAI_BASE_URL="http://127.0.0.1:9420/v1"
export OPENAI_API_KEY="sk-dg-abc123…"
# model in code or OPENAI_MODEL where supported:
#   client.chat.completions.create(model="gpt-5.4-mini", …)
#   client.embeddings.create(model="text-embedding-3-small", input="…")
```

**Embeddings (OpenAI SDK):**

```bash
curl http://127.0.0.1:9420/v1/embeddings \
  -H "Authorization: Bearer sk-dg-abc123…" \
  -H "Content-Type: application/json" \
  -d '{"model":"text-embedding-3-small","input":["hello world"]}'
```

**Anthropic SDK:**

```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:9420"
export ANTHROPIC_API_KEY="sk-dg-abc123…"
# client.messages.create(model="claude-sonnet-4-6", …)
```

**curl (OpenAI wire):**

```bash
curl http://127.0.0.1:9420/v1/chat/completions \
  -H "Authorization: Bearer sk-dg-abc123…" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.4-mini","messages":[{"role":"user","content":"hi"}],"stream":true}'
```

No `HTTP_PROXY`. No `CONNECT`. No client knowledge of upstream provider URLs.

---

## Model field semantics

| Field | Meaning on daigate |
|-------|----------------------|
| Request `model` | **Catalog id** (`gpt-5.4-mini`, `claude-sonnet-4-6`, …) |
| Catalog → provider `model` | **Upstream provider string** (`gpt-5.4-mini`, …) |
| Client env `OPENAI_MODEL` | Optional default catalog id (tool-dependent) |

The developer thinks in **one stable model namespace** you operate; daigate maps to vendor naming internally.

---

## Wire choice = SDK choice

Catalog entry declares which ingress wire serves a model:

```yaml
gpt-5.4-mini:
  modalities:
    chat:
      wire: openai-chat-completions   # use OPENAI_* env + OpenAI SDK

claude-sonnet-4-6:
  modalities:
    chat:
      wire: anthropic-messages        # use ANTHROPIC_* env + Anthropic SDK

text-embedding-3-small:
  modalities:
    embed:
      wire: openai-embeddings         # POST /v1/embeddings, same OPENAI_* env
      providers:
        - provider_ref: openai
          surface: embed
          model: text-embedding-3-small
```

**v1:** no automatic translation between chat wires. If a tool only speaks OpenAI chat completions, catalog that model with `wire: openai-chat-completions` and an OpenAI-compatible upstream.

---

## Internal naming: `upstream` not `forward`

daigate implements an **outbound relay** after ingress handling. In docs and packages, prefer **`upstream`** (or `relay`) for that hop — avoid calling the product a “forward proxy”.

| Term | Meaning |
|------|---------|
| **Ingress** | daigate-compatible `/v1/*` endpoints clients call |
| **Upstream** | HTTPS call from daigate to real provider (inject auth, stream back) |
| ~~Forward proxy~~ | **Dropped** public API — no `HTTP_PROXY`, no `/v1/forward`, no `CONNECT` |

```text
Client ──► ingress (/v1/chat/completions) ──► catalog ──► upstream (api.openai.com)
              ▲ gateway API key                    ▲ vault OAuth/api_key
```

---

## CI/CD — smoke tests

Mint on the **admin listener** (`:9421` by default), smoke on the **data plane** (`:9420`). CI needs network access to both (admin is often internal-only).

### GitLab / CI variables

| Variable | Value | Masked? |
|----------|-------|---------|
| `OPENAI_BASE_URL` | `http://daigate:9420/v1` | no |
| `OPENAI_MODEL` | catalog id, e.g. `gpt-5.4-mini` | no |
| `DAIGATE_ADMIN_URL` | `http://daigate:9421` | no |
| `DAIGATE_PROVISION_TOKEN` | provision role token (mint only) | **yes** |

### One curl — mint the gateway key

`POST /v1/keys` on the **admin listener**. Auth: **provision** token — cannot read upstream vault.

```bash
# before_script (GitLab, GitHub Actions, etc.)
export OPENAI_API_KEY=$(
  curl -fsS -X POST "${DAIGATE_ADMIN_URL}/v1/keys" \
    -H "Authorization: Bearer ${DAIGATE_PROVISION_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"ci-${CI_PIPELINE_ID:-local}\"}" \
  | jq -r .key
)
```

Response:

```json
{ "id": 42, "key": "sk-dg-42.…" }
```

Provision mints **`issued`** keys with scopes/TTL capped by `admin.provision` in yaml (default: chat wires, 24h).

```bash
curl -fsS "${OPENAI_BASE_URL%/v1}/v1/healthz"
curl -fsS "$OPENAI_BASE_URL/chat/completions" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"$OPENAI_MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}]}"
```

### GitLab example

```yaml
variables:
  OPENAI_BASE_URL: "http://daigate.internal:9420/v1"
  OPENAI_MODEL: "gpt-5.4-mini"
  DAIGATE_ADMIN_URL: "http://daigate.internal:9421"

smoke:
  before_script:
    - apk add --no-cache curl jq
    - |
      export OPENAI_API_KEY=$(curl -fsS -X POST "${DAIGATE_ADMIN_URL}/v1/keys" \
        -H "Authorization: Bearer ${DAIGATE_PROVISION_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"ci-${CI_PIPELINE_ID}\"}" | jq -r .key)
  script:
    - curl -fsS "$OPENAI_BASE_URL/chat/completions" -H "Authorization: Bearer $OPENAI_API_KEY" \
        -H "Content-Type: application/json" \
        -d "{\"model\":\"$OPENAI_MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}]}"
```

**Do not** put `DAIGATE_ADMIN_TOKEN` in CI. Provision token mints `issued` keys only.

Anthropic-wire tests: same pattern — set `ANTHROPIC_BASE_URL` / `ANTHROPIC_API_KEY=daigate` and extend bootstrap to export both, or use OpenAI wire models in smoke.

---

## Hosted / SaaS story

Tenants get gateway keys (admin-minted or via linked issuer plugins) — same `OPENAI_BASE_URL` + `OPENAI_API_KEY` + catalog `model`. They never receive upstream provider keys.

---

## Operator notes

- **Ingress auth** is required on any exposed data plane (loopback-only dev is the exception).
- **`GET /v1/models`** lists catalog model ids visible to the gateway key's scopes.
- **Embeddings** share `OPENAI_BASE_URL` — RAG clients use `POST /v1/embeddings` with catalog embed models ([catalog.md](catalog.md)).