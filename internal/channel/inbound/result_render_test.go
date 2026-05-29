package inbound

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/command"
)

func listResult(total, page, pageSize int) *command.Result {
	items := make([]command.ListItem, 0, pageSize)
	for i := 0; i < pageSize && i < total; i++ {
		items = append(items, command.ListItem{Label: "item", Detail: "d"})
	}
	return &command.Result{
		Text: "list text",
		Interactive: &command.Interactive{
			Kind: command.InteractiveList,
			List: &command.ListView{
				Resource: "mcp", Action: "list",
				Items: items, Total: total, Page: page, PageSize: pageSize,
			},
		},
	}
}

func TestRenderResultTextFallbackWhenNoButtons(t *testing.T) {
	res := listResult(50, 0, 12)
	msg := renderResult(res, channel.ChannelCapabilities{Buttons: false})
	if msg.Text != "list text" {
		t.Errorf("Text = %q, want 'list text'", msg.Text)
	}
	if len(msg.Actions) != 0 {
		t.Errorf("expected no actions for non-button channel, got %d", len(msg.Actions))
	}
}

func TestRenderResultTextFallbackWhenNoInteractive(t *testing.T) {
	msg := renderResult(&command.Result{Text: "plain"}, channel.ChannelCapabilities{Buttons: true})
	if msg.Text != "plain" || len(msg.Actions) != 0 {
		t.Errorf("got Text=%q actions=%d, want 'plain'/0", msg.Text, len(msg.Actions))
	}
}

func TestRenderListSinglePageNoButtons(t *testing.T) {
	msg := renderResult(listResult(5, 0, 12), channel.ChannelCapabilities{Buttons: true})
	if len(msg.Actions) != 0 {
		t.Errorf("single page should render no buttons, got %d", len(msg.Actions))
	}
}

func TestRenderListMultiPageNavButtons(t *testing.T) {
	// 50 items, 12/page => 5 pages. Page 0 should have: indicator, Next, Close.
	msg := renderResult(listResult(50, 0, 12), channel.ChannelCapabilities{Buttons: true})
	if len(msg.Actions) == 0 {
		t.Fatal("expected nav buttons on a multi-page list")
	}
	var hasNext, hasPrev, hasClose, hasIndicator bool
	navRow := -1
	for _, a := range msg.Actions {
		switch a.Label {
		case "Next ▶":
			hasNext = true
			navRow = a.Row
		case "◀ Prev":
			hasPrev = true
		case "✕ Close":
			hasClose = true
		case "1/5":
			hasIndicator = true
		}
	}
	if hasPrev {
		t.Error("page 0 should not have a Prev button")
	}
	if !hasNext || !hasClose || !hasIndicator {
		t.Errorf("missing buttons: next=%v close=%v indicator=%v", hasNext, hasClose, hasIndicator)
	}
	// Indicator and Next share the nav row; Close is on its own (later) row.
	for _, a := range msg.Actions {
		if a.Label == "1/5" && a.Row != navRow {
			t.Errorf("indicator row=%d, want nav row %d", a.Row, navRow)
		}
		if a.Label == "✕ Close" && a.Row <= navRow {
			t.Errorf("close row=%d should be after nav row %d", a.Row, navRow)
		}
	}
}

func TestRenderListLastPageHasPrevNotNext(t *testing.T) {
	// 50 items, 12/page, last page index = 4.
	msg := renderResult(listResult(50, 4, 12), channel.ChannelCapabilities{Buttons: true})
	var hasNext, hasPrev bool
	for _, a := range msg.Actions {
		switch a.Label {
		case "Next ▶":
			hasNext = true
		case "◀ Prev":
			hasPrev = true
		}
	}
	if hasNext {
		t.Error("last page should not have a Next button")
	}
	if !hasPrev {
		t.Error("last page should have a Prev button")
	}
}

func TestRenderModelPickerProviderGrid(t *testing.T) {
	res := &command.Result{
		Text: "models",
		Interactive: &command.Interactive{
			Kind: command.InteractiveModelPicker,
			Picker: &command.ModelPickerView{
				Level: command.LevelProviders,
				Providers: []command.PickerProvider{
					{Index: 0, Name: "Anthropic", HasCurrent: true},
					{Index: 1, Name: "OpenAI"},
					{Index: 2, Name: "DeepSeek"},
				},
				Page: 0, PageSize: 10, Total: 3,
			},
		},
	}
	msg := renderResult(res, channel.ChannelCapabilities{Buttons: true})

	// Provider buttons are 2 per row.
	provByRow := map[int]int{}
	var currentMarked bool
	for _, a := range msg.Actions {
		if a.Label == "✕ Close" {
			continue
		}
		provByRow[a.Row]++
		if strings.Contains(a.Label, "●") {
			currentMarked = true
		}
	}
	if !currentMarked {
		t.Error("expected ● marker on the provider holding the current model")
	}
	if provByRow[0] != 2 {
		t.Errorf("first row should have 2 provider buttons, got %d", provByRow[0])
	}
	// Provider taps drill into that provider's model list at page 0.
	if got := msg.Actions[0].Value; got != command.EncodeModelProviderCallback(0, 0) {
		t.Errorf("first provider callback = %q, want %q", got, command.EncodeModelProviderCallback(0, 0))
	}
}

func TestFormatNewSessionMessage(t *testing.T) {
	got := formatNewSessionMessage("chat", command.CurrentContext{
		ChatModel: "Claude Opus 4.7 (Anthropic)", HeartbeatModel: "DeepSeek V4 (DeepSeek)",
		ReasoningEnabled: true, ReasoningEffort: "medium",
	})
	for _, want := range []string{"New chat conversation started.", "Model: Claude Opus 4.7 (Anthropic)", "Heartbeat: DeepSeek V4 (DeepSeek)", "Reasoning: medium"} {
		if !strings.Contains(got, want) {
			t.Errorf("message missing %q:\n%s", want, got)
		}
	}

	off := formatNewSessionMessage("discuss", command.CurrentContext{ChatModel: "(none)", HeartbeatModel: "(none)", ReasoningEnabled: false})
	if !strings.Contains(off, "Reasoning: off") {
		t.Errorf("disabled reasoning should show 'off': %s", off)
	}
	if !strings.Contains(off, "New discuss conversation started.") {
		t.Errorf("mode label not reflected: %s", off)
	}
}

func TestRenderModelPickerModelLevel(t *testing.T) {
	// 20 models, 8/page => 3 pages. Page 1 (middle) should have Prev+Next,
	// a Back-to-providers button, ✓ on the selected model, and Close.
	models := make([]command.PickerModel, 0, 20)
	for i := 0; i < 20; i++ {
		models = append(models, command.PickerModel{FlatIndex: i, Name: "m", Selected: i == 9})
	}
	res := &command.Result{
		Text: "models",
		Interactive: &command.Interactive{
			Kind: command.InteractiveModelPicker,
			Picker: &command.ModelPickerView{
				Level: command.LevelModels, Models: models,
				ProviderIndex: 2, Page: 1, PageSize: 8, Total: 20,
			},
		},
	}
	msg := renderResult(res, channel.ChannelCapabilities{Buttons: true})

	var hasPrev, hasNext, hasBack, hasClose, hasSelected bool
	for _, a := range msg.Actions {
		switch a.Label {
		case "◀ Prev":
			hasPrev = true
		case "Next ▶":
			hasNext = true
		case "◀ Providers":
			hasBack = true
			if a.Value != command.EncodeListCallback("model", "list", nil, 0) {
				t.Errorf("back button callback = %q", a.Value)
			}
		case "✕ Close":
			hasClose = true
		}
		if strings.HasPrefix(a.Label, "✓ ") {
			hasSelected = true
		}
	}
	if !hasPrev || !hasNext {
		t.Errorf("middle page should have Prev and Next: prev=%v next=%v", hasPrev, hasNext)
	}
	if !hasBack || !hasClose {
		t.Errorf("model level should have Back and Close: back=%v close=%v", hasBack, hasClose)
	}
	// Selected model is flat index 9, which falls in page 1's slice [8,16).
	if !hasSelected {
		t.Error("selected model (flat 9) should be marked with ✓ on page 1")
	}
}
