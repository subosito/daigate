// Package observability provides ingress structured logs and optional OpenTelemetry export.
//
// Every data-plane request emits one JSON line on stderr (no secrets). Spans and
// metrics are noop until the process initializes OTel.
//
// Standalone binaries (daigate serve, operator examples) own OTLP export:
//
//	observability.Boot("daigate")
//	defer observability.ShutdownGraceful()
//
// Library embedders must not call Boot when the host already
// exports OTel. Call Hook after the host observability Boot:
//
//	observability.Hook("daigate")
//	gw.ListenAndServe(ctx) // does not Boot or Shutdown OTel
//
// Hook is idempotent. ShutdownGraceful is a no-op when Hooked().
// See docs/observability.md.
package observability