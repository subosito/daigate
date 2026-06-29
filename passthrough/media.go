package passthrough

import (
	"context"
	"io"
	"net/http"

	"github.com/subosito/daigate/adaptersdk/handler"
)

// ImageHandler relays image requests without transformation.
type ImageHandler struct {
	ProtocolName string
	WireID       string
}

func (h *ImageHandler) Protocol() string { return h.ProtocolName }

func (h *ImageHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, ingressPath string, body io.Reader, hdr http.Header) (*http.Response, error) {
	return relay(ctx, client, t, http.MethodPost, ingressPath, body, hdr)
}

// SpeechHandler relays TTS requests without transformation.
type SpeechHandler struct {
	ProtocolName string
	WireID       string
}

func (h *SpeechHandler) Protocol() string { return h.ProtocolName }

func (h *SpeechHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, body io.Reader, hdr http.Header) (*http.Response, error) {
	return relay(ctx, client, t, http.MethodPost, "/v1/audio/speech", body, hdr)
}

// VideoHandler relays video submit/poll without transformation.
type VideoHandler struct {
	ProtocolName string
	WireID       string
}

func (h *VideoHandler) Protocol() string { return h.ProtocolName }

func (h *VideoHandler) Forward(ctx context.Context, client *http.Client, t handler.Target, ingressPath string, body io.Reader, hdr http.Header) (*http.Response, error) {
	method := http.MethodPost
	if body == nil {
		method = http.MethodGet
	}
	return relay(ctx, client, t, method, ingressPath, body, hdr)
}