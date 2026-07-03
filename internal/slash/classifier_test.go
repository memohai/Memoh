package slash

import "testing"

func TestClassifyWebSkillUseRequiresChip(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:    "/skill use alpha -- do it",
		Surface: SurfaceWebWS,
	})
	if decision.Kind != DecisionReject || decision.Code != CodeUseSkillChipRequired {
		t.Fatalf("decision = %#v, want reject %s", decision, CodeUseSkillChipRequired)
	}
}

func TestClassifyChannelSkillUse(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:       "/skill@Memoh use alpha,beta -- do it",
		Surface:    SurfaceChannel,
		IsGroup:    true,
		BotAliases: []string{"Memoh"},
	})
	if decision.Kind != DecisionSkillIntent {
		t.Fatalf("kind = %s, want %s", decision.Kind, DecisionSkillIntent)
	}
	if !decision.Directed {
		t.Fatal("directed = false, want true")
	}
	if got := len(decision.SkillIntent.Names); got != 2 {
		t.Fatalf("names len = %d, want 2", got)
	}
	if decision.SkillIntent.Prompt != "do it" {
		t.Fatalf("prompt = %q", decision.SkillIntent.Prompt)
	}
}

func TestClassifyChannelSkillUseAllowsDoubleHyphenInName(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:    "/skill use my--skill -- do it",
		Surface: SurfaceChannel,
	})
	if decision.Kind != DecisionSkillIntent {
		t.Fatalf("kind = %s, want %s: %#v", decision.Kind, DecisionSkillIntent, decision)
	}
	if len(decision.SkillIntent.Names) != 1 || decision.SkillIntent.Names[0] != "my--skill" {
		t.Fatalf("names = %#v, want my--skill", decision.SkillIntent.Names)
	}
	if decision.SkillIntent.Prompt != "do it" {
		t.Fatalf("prompt = %q, want do it", decision.SkillIntent.Prompt)
	}
}

func TestClassifySkillUseSyntax(t *testing.T) {
	tests := []struct {
		name string
		text string
		code string
	}{
		{name: "missing delimiter", text: "/skill use alpha", code: CodeInvalidSkillSlashSyntax},
		{name: "missing prompt", text: "/skill use alpha -- ", code: CodeMissingPrompt},
		{name: "delimiter without leading space", text: "/skill use alpha-- do", code: CodeInvalidSkillSlashSyntax},
		{name: "delimiter without trailing space", text: "/skill use alpha --do", code: CodeInvalidSkillSlashSyntax},
		{name: "empty segment", text: "/skill use alpha,,beta -- do", code: CodeInvalidSkillSlashSyntax},
		{name: "dot", text: "/skill use . -- do", code: CodeInvalidSkillSlashSyntax},
		{name: "dot dot", text: "/skill use .. -- do", code: CodeInvalidSkillSlashSyntax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := Classify(ClassifyInput{Text: tt.text, Surface: SurfaceChannel, Directed: true})
			if decision.Kind != DecisionReject || decision.Code != tt.code {
				t.Fatalf("decision = %#v, want reject %s", decision, tt.code)
			}
		})
	}
}

func TestClassifyUndirectedGroupNoopBeatsAttachment(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:           "/help",
		Surface:        SurfaceChannel,
		IsGroup:        true,
		HasAttachments: true,
		KnownCommand:   func(resource string) bool { return resource == "help" },
	})
	if decision.Kind != DecisionRejectNoop {
		t.Fatalf("decision = %#v, want noop", decision)
	}
}

func TestClassifyDirectedAttachmentBeatsUnknown(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:           "/wat",
		Surface:        SurfaceChannel,
		Directed:       true,
		HasAttachments: true,
	})
	if decision.Kind != DecisionReject || decision.Code != CodeSlashAttachmentsUnsupported {
		t.Fatalf("decision = %#v, want attachment reject", decision)
	}
}

func TestClassifyWebKnownCommands(t *testing.T) {
	known := func(resource string) bool { return resource == "help" || resource == "start" }
	allowed := func(resource, action string) bool { return resource == "help" && action == "" }

	help := Classify(ClassifyInput{
		Text:               "/help",
		Surface:            SurfaceWebWS,
		KnownCommand:       known,
		WebActionSupported: allowed,
	})
	if help.Kind != DecisionCommandAction {
		t.Fatalf("help decision = %#v, want command action", help)
	}

	start := Classify(ClassifyInput{
		Text:               "/start",
		Surface:            SurfaceWebWS,
		KnownCommand:       known,
		WebActionSupported: allowed,
	})
	if start.Kind != DecisionUnsupportedCommand || start.Code != CodeUnsupportedWebCommand {
		t.Fatalf("start decision = %#v, want unsupported web command", start)
	}
}

func TestClassifyModeSlashRemainderRejects(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:         "/btw /help",
		Surface:      SurfaceChannel,
		Directed:     true,
		SupportsMode: true,
	})
	if decision.Kind != DecisionReject || decision.Code != CodeUnknownSlash {
		t.Fatalf("decision = %#v, want slash reject", decision)
	}
}

func TestClassifyUnknownSlash(t *testing.T) {
	decision := Classify(ClassifyInput{Text: "/wat", Surface: SurfaceWebWS})
	if decision.Kind != DecisionUnknownSlash || decision.Code != CodeUnknownSlash {
		t.Fatalf("decision = %#v, want unknown slash", decision)
	}
}
