package models

import (
	"net/http"
	"time"
)

const (
	DefaultProviderRequestTimeout      = 2 * time.Minute
	DefaultProviderProbeTimeout        = 60 * time.Second
	DefaultProviderTLSHandshakeTimeout = 30 * time.Second
)

var defaultProviderTransport = newDefaultProviderTransport()

// NewProviderHTTPClient returns an HTTP client for model/provider traffic.
// When timeout is zero or negative, the caller is expected to enforce limits
// via context deadlines, which keeps streaming responses unbounded by the
// client's global timeout while still using the relaxed TLS handshake window.
func NewProviderHTTPClient(timeout time.Duration) *http.Client {
	client := &http.Client{Transport: defaultProviderTransport}
	if timeout > 0 {
		client.Timeout = timeout
	}
	return client
}

func newDefaultProviderTransport() *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{TLSHandshakeTimeout: DefaultProviderTLSHandshakeTimeout}
	}

	transport := base.Clone()
	if transport.TLSHandshakeTimeout < DefaultProviderTLSHandshakeTimeout {
		transport.TLSHandshakeTimeout = DefaultProviderTLSHandshakeTimeout
	}
	return transport
}
