package upstream

import "strings"

// JoinURL appends an ingress wire path to a provider base_url.
// When base_url already ends with /v1 and path starts with /v1/, the duplicate
// segment is dropped so both catalog styles work:
//   - https://host/v1            + /v1/chat/completions → …/v1/chat/completions
//   - https://api.example.com    + /v1/chat/completions → …/v1/chat/completions
func JoinURL(baseURL, path string) string {
	base := strings.TrimRight(baseURL, "/")
	if path == "" || path == "/" {
		return base
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	return base + path
}