package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHTTPExecutorCountItemsDecodesJSON(t *testing.T) {
	e := &httpExecutor{cfg: defaultConfig()}
	rows, err := e.countItems([]byte(`{"items":[{"id":"1"},{"id":"2"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("rows = %d", rows)
	}
}

func TestHTTPExecutorCountItemsCanSkipDecode(t *testing.T) {
	cfg := defaultConfig()
	cfg.Workload.HTTPDecodeJSON = false
	e := &httpExecutor{cfg: cfg}
	rows, err := e.countItems([]byte(`not json`))
	if err != nil {
		t.Fatal(err)
	}
	if rows != int64(len(`not json`)) {
		t.Fatalf("rows = %d", rows)
	}
}

func TestNormalizeHTTPHandlerError(t *testing.T) {
	err := normalizeHTTPHandlerError(echo.NewHTTPError(409, "stale session head"))
	if err == nil || !strings.Contains(err.Error(), "http status 409") {
		t.Fatalf("err = %v", err)
	}
	plain := errors.New("plain")
	if got := normalizeHTTPHandlerError(plain); !errors.Is(got, plain) {
		t.Fatalf("plain error changed: %v", got)
	}
}
