package wecom

import (
	"testing"
	"time"
)

func TestCallbackContextCache_PutGet(t *testing.T) {
	cache := newCallbackContextCache(1 * time.Hour)
	cache.Put("m1", callbackContext{ReqID: "r1"})
	got, ok := cache.Get("m1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ReqID != "r1" {
		t.Fatalf("unexpected req id: %q", got.ReqID)
	}
}

func TestCallbackContextCache_Expires(t *testing.T) {
	cache := newCallbackContextCache(1 * time.Second)
	cache.Put("m1", callbackContext{
		ReqID:     "r1",
		CreatedAt: time.Now().Add(-2 * time.Second),
	})
	if _, ok := cache.Get("m1"); ok {
		t.Fatal("expected cache miss due to expiry")
	}
}
