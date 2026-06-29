package upstream

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/subosito/daigate/credential/inject"
	"github.com/subosito/daigate/credential/store"
	"github.com/subosito/daigate/observability"
)

// Hop-by-hop headers must not be relayed (RFC 7230).
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func removeHopByHopHeaders(h http.Header) {
	if c := h.Get("Connection"); c != "" {
		for _, f := range strings.Split(c, ",") {
			if f = textproto.TrimString(f); f != "" {
				h.Del(f)
			}
		}
	}
	for _, k := range hopByHopHeaders {
		h.Del(k)
	}
}

// Client performs outbound relay.
type Client struct {
	HTTP *http.Client
}

func NewClient() *Client {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: DefaultTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &Client{HTTP: &http.Client{Transport: tr, Timeout: 0}}
}

// Relay forwards request body to upstream URL with credential inject.
func (c *Client) Relay(ctx context.Context, baseURL, path string, mat store.Material, preset string, body io.Reader, headers http.Header) (*http.Response, error) {
	url := JoinURL(baseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	inject.CopyHeaders(req, headers)
	inject.Apply(mat, req, preset)
	return observability.HTTPDo(ctx, c.HTTP, req)
}

// CopyResponse streams upstream response to client, with flush for SSE.
func CopyResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response) error {
	h := resp.Header.Clone()
	removeHopByHopHeaders(h)
	for k, vals := range h {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if resp.Body == nil {
		return nil
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		return copySSE(ctx, w, resp.Body)
	}
	_, err := io.Copy(w, resp.Body)
	return err
}

func copySSE(ctx context.Context, w http.ResponseWriter, body io.Reader) error {
	flusher, ok := w.(http.Flusher)
	br := bufio.NewReader(body)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if _, werr := io.WriteString(w, line); werr != nil {
				return werr
			}
			if ok {
				flusher.Flush()
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// DrainReader reads body until EOF (for shutdown tests).
func DrainReader(ctx context.Context, r io.Reader) error {
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, r)
		done <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// DefaultTimeout for non-streaming posts.
const DefaultTimeout = 120 * time.Second