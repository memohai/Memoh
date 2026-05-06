package sso

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOIDCStateCookieRoundTripAndClear(t *testing.T) {
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	rec := httptest.NewRecorder()
	state := OIDCState{
		State:        "state-1",
		Nonce:        "nonce-1",
		CodeVerifier: "verifier-1",
	}

	if err := SetOIDCStateCookie(rec, state, now); err != nil {
		t.Fatalf("set cookie: %v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != OIDCStateCookieName {
		t.Fatalf("cookie name = %q", cookie.Name)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie flags not secure: httponly=%v secure=%v samesite=%v", cookie.HttpOnly, cookie.Secure, cookie.SameSite)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/sso/callback", nil)
	req.AddCookie(cookie)
	clearRec := httptest.NewRecorder()
	got, err := ReadAndClearOIDCStateCookie(clearRec, req, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("read cookie: %v", err)
	}
	if got.State != state.State || got.Nonce != state.Nonce || got.CodeVerifier != state.CodeVerifier {
		t.Fatalf("state mismatch: %#v", got)
	}
	clearCookies := clearRec.Result().Cookies()
	if len(clearCookies) != 1 || clearCookies[0].MaxAge != -1 {
		t.Fatalf("expected clear cookie, got %#v", clearCookies)
	}
}

func TestOIDCStateCookieExpired(t *testing.T) {
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	rec := httptest.NewRecorder()
	if err := SetOIDCStateCookie(rec, OIDCState{
		State:        "state-1",
		Nonce:        "nonce-1",
		CodeVerifier: "verifier-1",
		CreatedAt:    now.Add(-OIDCStateCookieTTL - time.Second),
	}, now); err != nil {
		t.Fatalf("set cookie: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/sso/callback", nil)
	req.AddCookie(rec.Result().Cookies()[0])
	if _, err := ReadAndClearOIDCStateCookie(httptest.NewRecorder(), req, now); err == nil {
		t.Fatal("expected expired cookie error")
	}
}
