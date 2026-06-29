package observability

import (
	"log/slog"
	"time"
)

// RequestLog fields for one ingress request (no secrets).
type RequestLog struct {
	Wire        string `json:"wire"`
	Model       string `json:"model"`
	ProviderRef string `json:"provider_ref"`
	Protocol    string `json:"protocol"`
	Status      int    `json:"status"`
	LatencyMs   int64  `json:"latency_ms"`
	PrincipalID string `json:"principal_id"`
}

// SetTestLogger redirects ingress logs (tests only).
func SetTestLogger(l *slog.Logger) {
	ingressLog = l
}

// LogRequest emits exactly one structured line per request (context-free; tests and legacy callers).
func LogRequest(wire, model, providerRef, protocol string, status int, start time.Time, principalID string) {
	RecordIngress(nil, &Recorder{
		Wire: wire, Model: model, ProviderRef: providerRef, Protocol: protocol, PrincipalID: principalID,
	}, status, start)
}