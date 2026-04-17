package models

import (
	"net/http"
	"testing"
	"time"
)

func TestNewProviderHTTPClientWithoutTimeoutKeepsStreamingFriendlyBehavior(t *testing.T) {
	client := NewProviderHTTPClient(0)
	if client == nil {
		t.Fatal("expected client")
		return
	}
	if client.Timeout != 0 {
		t.Fatalf("expected no client timeout, got %s", client.Timeout)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if transport.TLSHandshakeTimeout < DefaultProviderTLSHandshakeTimeout {
		t.Fatalf("expected TLS handshake timeout >= %s, got %s", DefaultProviderTLSHandshakeTimeout, transport.TLSHandshakeTimeout)
	}
}

func TestNewProviderHTTPClientWithTimeout(t *testing.T) {
	timeout := 45 * time.Second
	client := NewProviderHTTPClient(timeout)
	if client == nil {
		t.Fatal("expected client")
		return
	}
	if client.Timeout != timeout {
		t.Fatalf("expected timeout %s, got %s", timeout, client.Timeout)
	}
}
