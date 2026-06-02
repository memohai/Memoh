package command

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/i18n"
)

func TestItemActionTypeable(t *testing.T) {
	tests := []struct {
		name string
		a    *ItemAction
		want string
	}{
		{"nil receiver", nil, ""},
		{"empty resource", &ItemAction{Resource: "", Action: "list"}, ""},
		{"empty action", &ItemAction{Resource: "memory", Action: ""}, ""},
		{"resource and action only", &ItemAction{Resource: "memory", Action: "list"}, "/memory list"},
		{"single arg", &ItemAction{Resource: "memory", Action: "set", Args: []string{"alice"}}, "/memory set alice"},
		{"multiple args", &ItemAction{Resource: "model", Action: "set", Args: []string{"openai", "gpt-4o"}}, "/model set openai gpt-4o"},
		{"flag args", &ItemAction{Resource: "settings", Action: "update", Args: []string{"--heartbeat_enabled", "true"}}, "/settings update --heartbeat_enabled true"},
		{"empty arg skipped", &ItemAction{Resource: "memory", Action: "set", Args: []string{"alice", "", "  "}}, "/memory set alice"},
		{"whitespace trimmed", &ItemAction{Resource: " memory ", Action: " list "}, "/memory list"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Typeable(); got != tc.want {
				t.Errorf("Typeable() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFallbackTrailer_NilAndEmpty(t *testing.T) {
	loc := i18n.New("en")
	if got := FallbackTrailer(nil, loc); got != "" {
		t.Errorf("nil Interactive: got %q, want \"\"", got)
	}
	if got := FallbackTrailer(&Interactive{Kind: InteractiveList, List: nil}, loc); got != "" {
		t.Errorf("nil List view: got %q, want \"\"", got)
	}
	if got := FallbackTrailer(&Interactive{Kind: InteractiveChoices, Choices: nil}, loc); got != "" {
		t.Errorf("nil Choices view: got %q, want \"\"", got)
	}
	if got := FallbackTrailer(&Interactive{Kind: InteractiveModelPicker, Picker: nil}, loc); got != "" {
		t.Errorf("nil Picker view: got %q, want \"\"", got)
	}
	if got := FallbackTrailer(&Interactive{Kind: InteractiveRange, Range: nil}, loc); got != "" {
		t.Errorf("nil Range view: got %q, want \"\"", got)
	}
}

func TestFallbackTrailer_List(t *testing.T) {
	loc := i18n.New("en")

	tests := []struct {
		name     string
		iv       *Interactive
		contains []string // substrings that must appear in trailer
		empty    bool
	}{
		{
			name: "homogeneous switch list (memory)",
			iv: &Interactive{Kind: InteractiveList, List: &ListView{
				Resource: "memory", Action: "list",
				Items: []ListItem{
					{Label: "Alice", Action: &ItemAction{Resource: "memory", Action: "set", Args: []string{"Alice"}}},
					{Label: "Bob", Action: &ItemAction{Resource: "memory", Action: "set", Args: []string{"Bob"}}},
				},
			}},
			contains: []string{"Switch with", "/memory set <name>"},
		},
		{
			name: "display-only list no extras (heartbeat logs)",
			iv: &Interactive{Kind: InteractiveList, List: &ListView{
				Resource: "heartbeat", Action: "logs",
				Items: []ListItem{{Label: "10:00 OK"}, {Label: "11:00 OK"}},
			}},
			empty: true,
		},
		{
			name: "display-only with cross-nav extras (email)",
			iv: &Interactive{Kind: InteractiveList, List: &ListView{
				Resource: "email", Action: "providers",
				Items: []ListItem{{Label: "smtp.gmail.com"}},
				ExtraActions: []ListItem{
					{Label: "Bindings", Action: &ItemAction{Resource: "email", Action: "bindings"}},
					{Label: "Outbox", Action: &ItemAction{Resource: "email", Action: "outbox"}},
				},
			}},
			contains: []string{"Open:", "/email bindings", "/email outbox"},
		},
		{
			name: "HintVerb=details override (mcp)",
			iv: &Interactive{Kind: InteractiveList, List: &ListView{
				Resource: "mcp", Action: "list",
				HintVerb: HintVerbDetails,
				Items:    []ListItem{{Label: "server-a"}},
			}},
			contains: []string{"See details with", "/mcp get <name>"},
		},
		{
			name: "heterogeneous actions falls back to open",
			iv: &Interactive{Kind: InteractiveList, List: &ListView{
				Items: []ListItem{
					{Action: &ItemAction{Resource: "memory", Action: "list"}},
					{Action: &ItemAction{Resource: "search", Action: "list"}},
				},
			}},
			contains: []string{"Open:", "/memory list", "/search list"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FallbackTrailer(tc.iv, loc)
			if tc.empty {
				if got != "" {
					t.Errorf("got %q, want \"\"", got)
				}
				return
			}
			for _, sub := range tc.contains {
				if !strings.Contains(got, sub) {
					t.Errorf("trailer %q does not contain %q", got, sub)
				}
			}
		})
	}
}

func TestFallbackTrailer_Choices(t *testing.T) {
	loc := i18n.New("en")

	tests := []struct {
		name     string
		iv       *Interactive
		contains []string
		empty    bool
	}{
		{
			name: "SuppressFallback returns empty",
			iv: &Interactive{Kind: InteractiveChoices, Choices: &ChoicesView{
				SuppressFallback: true,
				Choices: []ListItem{
					{Action: &ItemAction{Resource: "schedule", Action: "list"}},
				},
			}},
			empty: true,
		},
		{
			name: "no actionable choices returns empty",
			iv: &Interactive{Kind: InteractiveChoices, Choices: &ChoicesView{
				Choices: []ListItem{{Label: "display only"}},
			}},
			empty: true,
		},
		{
			name: "homogeneous pick shape (reasoning levels)",
			iv: &Interactive{Kind: InteractiveChoices, Choices: &ChoicesView{
				Choices: []ListItem{
					{Action: &ItemAction{Resource: "reasoning", Action: "set", Args: []string{"off"}}},
					{Action: &ItemAction{Resource: "reasoning", Action: "set", Args: []string{"low"}}},
					{Action: &ItemAction{Resource: "reasoning", Action: "set", Args: []string{"high"}}},
				},
			}},
			contains: []string{"Pick with", "/reasoning set <off|low|high>"},
		},
		{
			name: "homogeneous toggle shape (heartbeat flags)",
			iv: &Interactive{Kind: InteractiveChoices, Choices: &ChoicesView{
				Choices: []ListItem{
					{Action: &ItemAction{Resource: "settings", Action: "update", Args: []string{"--heartbeat_enabled", "true"}}},
					{Action: &ItemAction{Resource: "settings", Action: "update", Args: []string{"--heartbeat_enabled", "false"}}},
				},
			}},
			contains: []string{"Toggle:", "/settings update --heartbeat_enabled true", "/settings update --heartbeat_enabled false"},
		},
		{
			name: "heterogeneous cross-nav (settings worst case)",
			iv: &Interactive{Kind: InteractiveChoices, Choices: &ChoicesView{
				Choices: []ListItem{
					{Action: &ItemAction{Resource: "settings", Action: "update", Args: []string{"--heartbeat_enabled", "true"}}},
					{Action: &ItemAction{Resource: "reasoning", Action: "show"}},
					{Action: &ItemAction{Resource: "model", Action: "list"}},
					{Action: &ItemAction{Resource: "memory", Action: "list"}},
				},
			}},
			contains: []string{"Open:", "/settings update --heartbeat_enabled true", "/reasoning show", "/model list", "/memory list"},
		},
		{
			name: "per-row Verb override wins over inference",
			iv: &Interactive{Kind: InteractiveChoices, Choices: &ChoicesView{
				Choices: []ListItem{
					{Action: &ItemAction{Resource: "reasoning", Action: "set", Args: []string{"low"}, Verb: HintVerbMenu}},
				},
			}},
			contains: []string{"Or type", "/reasoning set low"},
		},
		{
			name: "homogeneous single no-arg button (WithButtons empty state)",
			iv: &Interactive{Kind: InteractiveChoices, Choices: &ChoicesView{
				Choices: []ListItem{
					{Action: &ItemAction{Resource: "help", Action: "mcp"}},
				},
			}},
			contains: []string{"Or type", "/help mcp"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FallbackTrailer(tc.iv, loc)
			if tc.empty {
				if got != "" {
					t.Errorf("got %q, want \"\"", got)
				}
				return
			}
			for _, sub := range tc.contains {
				if !strings.Contains(got, sub) {
					t.Errorf("trailer %q does not contain %q", got, sub)
				}
			}
		})
	}
}

func TestFallbackTrailer_ModelPicker(t *testing.T) {
	loc := i18n.New("en")

	t.Run("LevelProviders", func(t *testing.T) {
		got := FallbackTrailer(&Interactive{
			Kind:   InteractiveModelPicker,
			Picker: &ModelPickerView{Level: LevelProviders},
		}, loc)
		for _, sub := range []string{"Or type", "/model list <provider_name>"} {
			if !strings.Contains(got, sub) {
				t.Errorf("trailer %q does not contain %q", got, sub)
			}
		}
	})

	t.Run("LevelModels", func(t *testing.T) {
		got := FallbackTrailer(&Interactive{
			Kind:   InteractiveModelPicker,
			Picker: &ModelPickerView{Level: LevelModels},
		}, loc)
		for _, sub := range []string{"Or type", "/model set <name>"} {
			if !strings.Contains(got, sub) {
				t.Errorf("trailer %q does not contain %q", got, sub)
			}
		}
	})
}

func TestFallbackTrailer_Range(t *testing.T) {
	loc := i18n.New("en")

	t.Run("normal", func(t *testing.T) {
		got := FallbackTrailer(&Interactive{
			Kind: InteractiveRange,
			Range: &RangeView{
				Resource: "usage",
				Action:   "summary",
				Current:  "7d",
				Presets:  []string{"24h", "7d", "30d", "all"},
			},
		}, loc)
		for _, sub := range []string{"Time window:", "/usage summary --range <preset>", "24h", "7d", "30d", "all"} {
			if !strings.Contains(got, sub) {
				t.Errorf("trailer %q does not contain %q", got, sub)
			}
		}
	})

	t.Run("missing resource returns empty", func(t *testing.T) {
		got := FallbackTrailer(&Interactive{
			Kind:  InteractiveRange,
			Range: &RangeView{Action: "summary", Presets: []string{"24h"}},
		}, loc)
		if got != "" {
			t.Errorf("got %q, want \"\"", got)
		}
	})
}

func TestFallbackTrailer_NoPageZeroArtifact(t *testing.T) {
	// Regression guard for the SyntheticCommand --page 0 artifact: trailers
	// must never emit "--page 0" since the typeable form is for users to type,
	// not for the callback decoder to round-trip.
	loc := i18n.New("en")
	cases := []*Interactive{
		{Kind: InteractiveList, List: &ListView{Resource: "memory", Action: "list", Items: []ListItem{
			{Action: &ItemAction{Resource: "memory", Action: "set", Args: []string{"Alice"}}},
		}}},
		{Kind: InteractiveChoices, Choices: &ChoicesView{Choices: []ListItem{
			{Action: &ItemAction{Resource: "reasoning", Action: "set", Args: []string{"low"}}},
		}}},
		{Kind: InteractiveModelPicker, Picker: &ModelPickerView{Level: LevelProviders}},
		{Kind: InteractiveRange, Range: &RangeView{Resource: "usage", Action: "summary", Presets: []string{"24h"}}},
	}
	for i, iv := range cases {
		got := FallbackTrailer(iv, loc)
		if strings.Contains(got, "--page 0") {
			t.Errorf("case %d trailer leaked --page 0: %q", i, got)
		}
	}
}

func TestFallbackTrailer_LocaleFallback(t *testing.T) {
	// A non-existent zh translation must fall back to en; never the raw key.
	loc := i18n.New("zh")
	got := FallbackTrailer(&Interactive{
		Kind: InteractiveList,
		List: &ListView{Items: []ListItem{
			{Action: &ItemAction{Resource: "memory", Action: "set", Args: []string{"x"}}},
		}},
	}, loc)
	if got == "" || strings.HasPrefix(got, "cmd.fallback.") {
		t.Errorf("zh trailer fell through to raw key or empty: %q", got)
	}
}
