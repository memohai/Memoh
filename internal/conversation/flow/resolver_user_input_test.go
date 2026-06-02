package flow

import (
	"testing"

	"github.com/memohai/memoh/internal/userinput"
)

func TestResponseOptionValueSkipsTextInputOptions(t *testing.T) {
	req := userinput.Request{
		UIPayload: userinput.UIPayload{
			Options: []userinput.UIOption{
				{ID: "a", Label: "Plan A", Value: "A"},
				{ID: "custom", Label: "Custom answer", Value: "custom", InputType: "text"},
			},
		},
	}

	if optionRequiresText(req, "a") {
		t.Fatal("fixed option should not require text")
	}
	if !optionRequiresText(req, "custom") {
		t.Fatal("text option should require text")
	}
	if value := optionValue(req, "a"); value != "A" {
		t.Fatalf("fixed option value = %#v", value)
	}
	if value := responseOptionValue(req, "custom"); value != nil {
		t.Fatalf("text option response value = %#v", value)
	}
}
