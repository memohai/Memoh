package command

import (
	"strings"
	"testing"
)

func TestEncodeDecodeListCallbackRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		action   string
		args     []string
		page     int
	}{
		{"no args page 0", "mcp", "list", nil, 0},
		{"no args page 3", "schedule", "list", nil, 3},
		{"single arg", "model", "list", []string{"openrouter"}, 2},
		{"multi arg", "model", "list", []string{"open", "router"}, 5},
		{"high page", "memory", "list", nil, 9999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := EncodeListCallback(tt.resource, tt.action, tt.args, tt.page)
			if len(data) > telegramCallbackLimit {
				t.Fatalf("callback_data %q exceeds %d bytes (%d)", data, telegramCallbackLimit, len(data))
			}
			parsed, ok := DecodeCallback(data)
			if !ok {
				t.Fatalf("DecodeCallback(%q) returned ok=false", data)
			}
			if parsed.Kind != callbackKindListPage {
				t.Errorf("Kind = %q, want %q", parsed.Kind, callbackKindListPage)
			}
			if parsed.Resource != tt.resource {
				t.Errorf("Resource = %q, want %q", parsed.Resource, tt.resource)
			}
			if parsed.Action != tt.action {
				t.Errorf("Action = %q, want %q", parsed.Action, tt.action)
			}
			if parsed.Page != tt.page {
				t.Errorf("Page = %d, want %d", parsed.Page, tt.page)
			}
			if strings.Join(parsed.Args, " ") != strings.Join(tt.args, " ") {
				t.Errorf("Args = %v, want %v", parsed.Args, tt.args)
			}
		})
	}
}

func TestEncodeListCallbackLongArgsUsesToken(t *testing.T) {
	longArg := strings.Repeat("very-long-provider-name-", 5) // ~120 chars
	data := EncodeListCallback("model", "list", []string{longArg}, 1)
	if len(data) > telegramCallbackLimit {
		t.Fatalf("callback_data %q exceeds %d bytes (%d)", data, telegramCallbackLimit, len(data))
	}
	if !strings.Contains(data, "~#") {
		t.Fatalf("expected stashed token form (~#...), got %q", data)
	}
	parsed, ok := DecodeCallback(data)
	if !ok {
		t.Fatalf("DecodeCallback(%q) returned ok=false", data)
	}
	if strings.Join(parsed.Args, " ") != longArg {
		t.Errorf("Args = %v, want [%q]", parsed.Args, longArg)
	}
}

func TestDecodeArgsTokenMiss(t *testing.T) {
	// A token that was never stashed should decode to nil (treated as unfiltered).
	if got := decodeArgsToken("#deadbeef"); got != nil {
		t.Errorf("decodeArgsToken(miss) = %v, want nil", got)
	}
}

func TestDecodeCallbackDismissAndNoop(t *testing.T) {
	if p, ok := DecodeCallback(DismissCallback()); !ok || p.Kind != callbackKindDismiss {
		t.Errorf("dismiss decode = %+v ok=%v", p, ok)
	}
	if p, ok := DecodeCallback(NoopCallback()); !ok || p.Kind != callbackKindNoop {
		t.Errorf("noop decode = %+v ok=%v", p, ok)
	}
}

func TestDecodeCallbackRejectsNonInteractive(t *testing.T) {
	for _, data := range []string{"approve:abc123", "reject:xyz", "random", ""} {
		if _, ok := DecodeCallback(data); ok {
			t.Errorf("DecodeCallback(%q) ok=true, want false", data)
		}
		if IsInteractiveCallback(data) {
			t.Errorf("IsInteractiveCallback(%q) = true, want false", data)
		}
	}
}

func TestSyntheticCommand(t *testing.T) {
	p := ParsedCallback{Kind: callbackKindListPage, Resource: "model", Action: "list", Args: []string{"openrouter"}, Page: 2}
	if got, want := p.SyntheticCommand(), "/model list openrouter --page 2"; got != want {
		t.Errorf("SyntheticCommand = %q, want %q", got, want)
	}
	noArgs := ParsedCallback{Kind: callbackKindListPage, Resource: "mcp", Action: "list", Page: 0}
	if got, want := noArgs.SyntheticCommand(), "/mcp list --page 0"; got != want {
		t.Errorf("SyntheticCommand = %q, want %q", got, want)
	}
	if got := (ParsedCallback{Kind: callbackKindDismiss}).SyntheticCommand(); got != "" {
		t.Errorf("dismiss SyntheticCommand = %q, want empty", got)
	}
}

// TestCallbackToCommandRoundTrip validates the full pagination round-trip:
// encode a button -> decode the tap -> build a synthetic command -> Parse it
// back into the same resource/action/args/page the renderer started from.
func TestCallbackToCommandRoundTrip(t *testing.T) {
	tests := []struct {
		resource, action string
		args             []string
		page             int
	}{
		{"mcp", "list", nil, 1},
		{"model", "list", []string{"openrouter"}, 4},
		{"schedule", "list", nil, 0},
	}
	for _, tt := range tests {
		data := EncodeListCallback(tt.resource, tt.action, tt.args, tt.page)
		parsed, ok := DecodeCallback(data)
		if !ok {
			t.Fatalf("DecodeCallback(%q) ok=false", data)
		}
		cmd := parsed.SyntheticCommand()
		reparsed, err := Parse(cmd)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", cmd, err)
		}
		if reparsed.Resource != tt.resource || reparsed.Action != tt.action || reparsed.Page != tt.page {
			t.Errorf("round-trip %q -> %+v, want resource=%s action=%s page=%d",
				cmd, reparsed, tt.resource, tt.action, tt.page)
		}
		if strings.Join(reparsed.Args, " ") != strings.Join(tt.args, " ") {
			t.Errorf("round-trip args = %v, want %v", reparsed.Args, tt.args)
		}
	}
}

func TestModelProviderCallbackRoundTrip(t *testing.T) {
	for _, tc := range []struct{ prov, page int }{{0, 0}, {3, 2}, {12, 9}} {
		data := EncodeModelProviderCallback(tc.prov, tc.page)
		if len(data) > telegramCallbackLimit {
			t.Fatalf("callback %q exceeds limit", data)
		}
		parsed, ok := DecodeCallback(data)
		if !ok || parsed.Kind != callbackKindModelProvider {
			t.Fatalf("decode %q -> %+v ok=%v", data, parsed, ok)
		}
		if parsed.ProviderIndex != tc.prov || parsed.Page != tc.page {
			t.Errorf("decoded prov=%d page=%d, want %d/%d", parsed.ProviderIndex, parsed.Page, tc.prov, tc.page)
		}
		reparsed, err := Parse(parsed.SyntheticCommand())
		if err != nil {
			t.Fatalf("Parse(%q): %v", parsed.SyntheticCommand(), err)
		}
		if reparsed.Resource != "model" || reparsed.Action != "list" || reparsed.Prov != tc.prov || reparsed.Page != tc.page {
			t.Errorf("synthetic re-parse = %+v, want model/list prov=%d page=%d", reparsed, tc.prov, tc.page)
		}
	}
}

func TestModelSelectCallbackRoundTrip(t *testing.T) {
	data := EncodeModelSelectCallback(17)
	parsed, ok := DecodeCallback(data)
	if !ok || parsed.Kind != callbackKindModelSelect || parsed.FlatIndex != 17 {
		t.Fatalf("decode %q -> %+v ok=%v", data, parsed, ok)
	}
	reparsed, err := Parse(parsed.SyntheticCommand())
	if err != nil {
		t.Fatalf("Parse(%q): %v", parsed.SyntheticCommand(), err)
	}
	if reparsed.Resource != "model" || reparsed.Action != "set" || reparsed.Flat != 17 {
		t.Errorf("synthetic re-parse = %+v, want model/set flat=17", reparsed)
	}
}
