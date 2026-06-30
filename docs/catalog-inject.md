# Catalog inject — upstream auth headers

How `providers.yaml` tells the gateway which headers to set on outbound upstream requests after credentials are loaded from the vault.

**Secrets** live in the credential store (`credential_profile` → `broker.db`). **Inject** describes how those secrets become HTTP headers.

---

## Resolution order

| Priority | Source | When to use |
|----------|--------|-------------|
| 1 | `providers.*.inject` map | One or more headers — custom names, multi-header OAuth |
| 2 | `providers.*.inject_preset` | `bearer` or `x-api-key` |
| 3 | Adapter default (`inject.Spec` in code) | Linked translate adapters |
| 4 | Core default | `bearer` |

When `inject:` is set, `inject_preset` is skipped.

Client-supplied `Authorization` and `x-api-key` are stripped before inject.

---

## `inject:` — header map

Flat map: header name → template. Placeholders:

| Placeholder | API key | OAuth |
|-------------|---------|-------|
| `${key}` | `Material.APIKey` | — |
| `${access}` | — | `Material.AccessToken` |
| `${accountId}` | — | `Material.Extra("account_id")` |
| `${projectId}` | — | `Material.Extra("project_id")` |

```yaml
providers:
  acme-api-key:
    credential_profile: acme
    inject:
      x-vendor-api-key: "${key}"
    surfaces:
      default:
        protocol: openai-chat-completions
        base_url: https://api.acme.com

  acme-oauth:
    credential_profile: acme-oauth
    inject:
      authorization: "Bearer ${access}"
      x-account-id: "${accountId}"
    surfaces:
      default:
        protocol: openai-chat-completions
        base_url: https://api.acme.com
```

---

## `inject_preset` — bearer and x-api-key

| Value | API key upstream |
|-------|------------------|
| `bearer` (default) | `Authorization: Bearer ${key}` |
| `x-api-key` | `x-api-key: ${key}` |

```yaml
providers:
  acme:
    credential_profile: acme
    inject_preset: x-api-key
    surfaces:
      default:
        protocol: openai-chat-completions
        base_url: https://api.acme.com
```

OAuth defaults to bearer. Extra OAuth headers use `inject:` map.

---

## Adapter defaults

Translate adapters may ship a default `inject.Spec` when yaml omits both fields — e.g. a custom API-key header name in adapter code.

Override when needed:

```yaml
inject:
  x-vendor-api-key: "${key}"
```

---

## broker.db vs providers.yaml

| Concern | Where |
|---------|--------|
| Secret values | Credential store / `broker.db` |
| Profile binding | `providers.*.credential_profile` |
| Header templates | `providers.*.inject` or `inject_preset` (`bearer` \| `x-api-key`) |

---

## See also

- [auth.md](auth.md) — vault, `Material.Extras`
- [catalog.md](catalog.md) — provider surfaces and model pools