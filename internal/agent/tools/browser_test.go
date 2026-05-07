package tools

import "testing"

func TestBrowserKeyChordHelpers(t *testing.T) {
	parts := splitKeyChord("Control+Shift+a")
	if len(parts) != 3 || parts[0] != "Control" || parts[1] != "Shift" || parts[2] != "a" {
		t.Fatalf("unexpected chord parts: %#v", parts)
	}
	if got := namedKeysym("Enter"); got != 0xff0d {
		t.Fatalf("unexpected Enter keysym: %#x", got)
	}
	if got := namedKeysym("Control"); got != 0xffe3 {
		t.Fatalf("unexpected Control keysym: %#x", got)
	}
	if got := keysymForRune('你'); got != 0x01000000|uint32('你') {
		t.Fatalf("unexpected unicode keysym: %#x", got)
	}
}

func TestBrowserCDPKeyInfo(t *testing.T) {
	enter := keyInfoForCDP("Enter")
	if enter.Key != "Enter" || enter.KeyCode != 13 {
		t.Fatalf("unexpected Enter key info: %#v", enter)
	}
	letter := keyInfoForCDP("a")
	if letter.Key != "a" || letter.Code != "KeyA" || letter.KeyCode != int('A') || letter.Text != "a" {
		t.Fatalf("unexpected letter key info: %#v", letter)
	}
	if got := cdpModifier("Control") | cdpModifier("Shift"); got != 10 {
		t.Fatalf("unexpected modifier mask: %d", got)
	}
}

func TestBrowserScrollDeltas(t *testing.T) {
	if got := scrollDeltaY("down", 500); got != 500 {
		t.Fatalf("unexpected down delta: %d", got)
	}
	if got := scrollDeltaY("up", 500); got != -500 {
		t.Fatalf("unexpected up delta: %d", got)
	}
	if got := scrollDeltaX("left", 300); got != -300 {
		t.Fatalf("unexpected left delta: %d", got)
	}
	if got := scrollDeltaX("right", 300); got != 300 {
		t.Fatalf("unexpected right delta: %d", got)
	}
}

func TestBrowserActionAliases(t *testing.T) {
	if got := normalizeBrowserAction("dblclick"); got != "double_click" {
		t.Fatalf("unexpected dblclick alias: %q", got)
	}
	if got := normalizeBrowserAction("scrollintoview"); got != "scroll_into_view" {
		t.Fatalf("unexpected scrollintoview alias: %q", got)
	}
	if got := normalizeBrowserAction("fill"); got != "fill" {
		t.Fatalf("unexpected canonical action: %q", got)
	}
}

func TestBrowserRefHelpers(t *testing.T) {
	for _, input := range []string{"12", "e12", "E12", "ref=e12"} {
		if got := normalizeBrowserRef(input); got != "e12" {
			t.Fatalf("normalizeBrowserRef(%q) = %q", input, got)
		}
	}
	if _, err := browserRefIndex("e0"); err == nil {
		t.Fatal("expected invalid zero ref")
	}
	target := browserTargetArg(map[string]any{"ref": "12", "selector": "#fallback"}, "selector", "ref")
	if target.Ref != "e12" || target.Selector != "#fallback" {
		t.Fatalf("unexpected target: %#v", target)
	}
	result := target.withResult(map[string]any{"ok": true})
	if result["ref"] != "e12" || result["selector"] != "#fallback" {
		t.Fatalf("target metadata missing from result: %#v", result)
	}
}

func TestBrowserSchemasAreStrict(t *testing.T) {
	schema := browserObjectSchema(map[string]any{"action": map[string]any{"type": "string"}}, []string{"action"})
	if schema["additionalProperties"] != false {
		t.Fatalf("expected strict browser schema, got %#v", schema["additionalProperties"])
	}
	if required, ok := schema["required"].([]string); !ok || len(required) != 1 || required[0] != "action" {
		t.Fatalf("unexpected required fields: %#v", schema["required"])
	}
}
