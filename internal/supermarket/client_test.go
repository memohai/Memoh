package supermarket

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientPinsRequestsToConfiguredOrigin(t *testing.T) {
	var requested string
	client := NewClient("https://supermarket.example", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requested = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("{}")),
			Request:    req,
			Header:     make(http.Header),
		}, nil
	})})
	resp, err := client.Get(context.Background(), "/api/packages?q=docs", "application/json")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	_ = resp.Body.Close()
	if requested != "https://supermarket.example/api/packages?q=docs" {
		t.Fatalf("requested URL = %q", requested)
	}
	artifactResp, err := client.GetArtifact(context.Background(), "https://attacker.example/artifact", "application/gzip")
	if artifactResp != nil {
		_ = artifactResp.Body.Close()
	}
	if !errors.Is(err, ErrCrossOrigin) {
		t.Fatalf("cross-origin Artifact error = %v, want ErrCrossOrigin", err)
	}
}

func TestClientForwardsHeadersAndRejectsCrossOriginRedirect(t *testing.T) {
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(redirectTarget.Close)

	var conditionalHeader string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conditionalHeader = req.Header.Get("If-None-Match")
		http.Redirect(w, req, redirectTarget.URL+"/artifact", http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	client := NewClient(origin.URL, origin.Client())
	headers := make(http.Header)
	headers.Set("If-None-Match", `"digest"`)
	resp, err := client.GetWithHeaders(context.Background(), "/api/artifacts/icon/digest", "image/png", headers)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrCrossOrigin) {
		t.Fatalf("cross-origin redirect error = %v, want ErrCrossOrigin", err)
	}
	if conditionalHeader != `"digest"` {
		t.Fatalf("If-None-Match = %q, want forwarded value", conditionalHeader)
	}
}

func TestClientRejectsInvalidBaseURL(t *testing.T) {
	client := NewClient("file:///tmp/supermarket", nil)
	resp, err := client.Get(context.Background(), "/api/packages", "application/json")
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrInvalidBaseURL) {
		t.Fatalf("invalid base URL error = %v, want ErrInvalidBaseURL", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
