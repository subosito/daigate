# Observability

Ingress telemetry for daigate — structured logs always; OpenTelemetry when standalone export or library hook is active.

All hooks live in `github.com/subosito/daigate/observability`. Billing ledger stays in the host app — OTel is ops/SLOs only.

---

## Default (no OTel init)

Every data-plane HTTP request emits **one JSON line** on stderr (no secrets):

`wire`, `model`, `provider_ref`, `protocol`, `status`, `latency_ms`, `principal_id`

Spans and metrics are noop until `Boot` or `Hook`. Enough for dev `grep`; enable OTLP for dashboards and correlated debugging.

`gateway.ListenAndServe` does **not** call `Boot` — avoids fighting embedder global OTel state.

---

## Standalone (`daigate serve`)

`cmd/daigate/serve` and integrator operator binaries call:

```go
observability.Boot("daigate")
defer observability.ShutdownGraceful()
```

Set env before serve:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
export OTEL_SERVICE_NAME=daigate   # optional; overrides Boot default name
```

daigate owns OTLP export (traces, metrics, logs) and `slog` bridge.

When `OTEL_EXPORTER_OTLP_ENDPOINT` is unset: stderr JSON only, zero OTLP overhead.

| Variable | Role |
|----------|------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Collector base URL (e.g. `http://127.0.0.1:4318`) |
| `OTEL_EXPORTER_OTLP_HEADERS` | `key=value` pairs for auth |
| `OTEL_SERVICE_NAME` | Defaults to `daigate` |
| `OTEL_SDK_DISABLED=true` | Force noop |

No `daigate.yaml` observability block — env vars match the OTel ecosystem. Use any OTLP-capable collector (Vector, Grafana Alloy, Jaeger, Datadog agent, …).

---

## Library embed (host app, custom binary)

**Do not call `Boot` in the embedder** if the host already exports OTel. Call **`Hook`** after host observability init:

```go
import (
    hostobs "example.com/myapp/observability"   // your OTLP Boot package
    daigateobs "github.com/subosito/daigate/observability"
)

hostobs.Boot("my-gateway")
defer hostobs.ShutdownGraceful()

daigateobs.Hook("daigate")

gw, _ := gateway.New(...)
gw.ListenAndServe(ctx)   // does not Boot or Shutdown OTel
```

| | Standalone `Boot` | Library `Hook` |
|--|-------------------|----------------|
| OTLP exporters | daigate creates | embedder owns |
| `otel.SetTracerProvider` | daigate sets | embedder already set |
| `slog.SetDefault` | daigate bridges OTLP | embedder owns |
| `daigate.*` metrics / spans | yes | yes (via global providers) |
| `service.name` resource | `OTEL_SERVICE_NAME` or `daigate` | from embedder `Boot` |

Use the **same** `OTEL_EXPORTER_OTLP_ENDPOINT` in the process — one collector, one resource.

`Hook` is idempotent. `ShutdownGraceful` on daigate is a no-op when `Hooked()`.

Without `Hook` and without `Boot`: stderr ingress JSON only.

---

## Instruments

| Signal | Name / shape |
|--------|----------------|
| Trace | `ingress.forward`; child `upstream.http` |
| Metrics | `daigate.ingress.requests`, `daigate.ingress.duration_ms`, `daigate.upstream.*` |
| Logs (standalone OTel on) | stderr JSON + OTLP via `otelslog` |
| Logs (library hook) | stderr JSON; app logs via embedder `slog` |

Instrumentation: `IngressLog` middleware + `wire/` after auth and catalog resolve. `HTTPDo` in `upstream/` and adapters adds child spans.

`X-Correlation-Id` and W3C `traceparent` propagate on ingress.

---

## Cardinality and security

**Emit (low cardinality):** `wire`, `status` / `status_class`, `provider_ref`, `protocol`, `principal_id`, `correlation_id`.

**Optional / gated:** catalog `model` on metrics (fine self-hosted; disable for high-cardinality multi-tenant).

**Never emit:** api keys, OAuth tokens, refresh tokens, `Authorization`, upstream secret bodies, credential vault payloads. See [security.md](security.md).

---

## Billing vs observability

Token counters on OTel metrics (when parseable from upstream responses) are **observability**, not billing truth.

---

## Package API

| API | Role |
|-----|------|
| `IngressLog` | Middleware — one record per HTTP request |
| `Recorder` | Context fields filled by `wire/` after resolve |
| `LogRequest` | Structured slog line (preserved when OTel off) |
| `Boot` / `ShutdownGraceful` | Standalone OTLP lifecycle |
| `Hook` / `Hooked` | Library attach to global providers |