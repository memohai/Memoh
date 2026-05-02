package sso

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type OIDCState struct {
	State        string    `json:"state"`
	Nonce        string    `json:"nonce"`
	CodeVerifier string    `json:"code_verifier"`
	CreatedAt    time.Time `json:"created_at"`
}

type SAMLState struct {
	RelayState string    `json:"relay_state"`
	RequestID  string    `json:"request_id"`
	CreatedAt  time.Time `json:"created_at"`
}

func SetOIDCStateCookie(w http.ResponseWriter, value OIDCState, now time.Time) error {
	if value.State == "" || value.Nonce == "" || value.CodeVerifier == "" {
		return ErrInvalidProfile
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now.UTC()
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     OIDCStateCookieName,
		Value:    base64.RawURLEncoding.EncodeToString(payload),
		Path:     "/",
		Expires:  now.UTC().Add(OIDCStateCookieTTL),
		MaxAge:   int(OIDCStateCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func ReadAndClearOIDCStateCookie(w http.ResponseWriter, r *http.Request, now time.Time) (OIDCState, error) {
	clearOIDCStateCookie(w, now)
	cookie, err := r.Cookie(OIDCStateCookieName)
	if err != nil {
		return OIDCState{}, err
	}
	payload, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return OIDCState{}, err
	}
	var value OIDCState
	if err := json.Unmarshal(payload, &value); err != nil {
		return OIDCState{}, err
	}
	if value.State == "" || value.Nonce == "" || value.CodeVerifier == "" {
		return OIDCState{}, ErrInvalidProfile
	}
	if value.CreatedAt.IsZero() || now.Sub(value.CreatedAt) > OIDCStateCookieTTL {
		return OIDCState{}, errors.New("sso: oidc state expired")
	}
	return value, nil
}

func clearOIDCStateCookie(w http.ResponseWriter, now time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     OIDCStateCookieName,
		Value:    "",
		Path:     "/",
		Expires:  now.UTC().Add(-time.Hour),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func SetSAMLStateCookie(w http.ResponseWriter, value SAMLState, now time.Time) error {
	if value.RelayState == "" || value.RequestID == "" {
		return ErrInvalidProfile
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now.UTC()
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SAMLStateCookieName,
		Value:    base64.RawURLEncoding.EncodeToString(payload),
		Path:     "/",
		Expires:  now.UTC().Add(SAMLStateCookieTTL),
		MaxAge:   int(SAMLStateCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func ReadAndClearSAMLStateCookie(w http.ResponseWriter, r *http.Request, now time.Time) (SAMLState, error) {
	clearSAMLStateCookie(w, now)
	cookie, err := r.Cookie(SAMLStateCookieName)
	if err != nil {
		return SAMLState{}, err
	}
	payload, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return SAMLState{}, err
	}
	var value SAMLState
	if err := json.Unmarshal(payload, &value); err != nil {
		return SAMLState{}, err
	}
	if value.RelayState == "" || value.RequestID == "" {
		return SAMLState{}, ErrInvalidProfile
	}
	if value.CreatedAt.IsZero() || now.Sub(value.CreatedAt) > SAMLStateCookieTTL {
		return SAMLState{}, errors.New("sso: saml state expired")
	}
	return value, nil
}

func clearSAMLStateCookie(w http.ResponseWriter, now time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SAMLStateCookieName,
		Value:    "",
		Path:     "/",
		Expires:  now.UTC().Add(-time.Hour),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
