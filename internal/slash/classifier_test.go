package slash

import "testing"

func TestClassifyDirectSkillActivation(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:    "/flutter-adding-home-screen-widgets add widgets",
		Surface: SurfaceWebWS,
	})
	if decision.Kind != DecisionSkillIntent {
		t.Fatalf("kind = %s, want %s", decision.Kind, DecisionSkillIntent)
	}
	if len(decision.SkillIntent.Names) != 1 || decision.SkillIntent.Names[0] != "flutter-adding-home-screen-widgets" {
		t.Fatalf("names = %#v, want flutter skill", decision.SkillIntent.Names)
	}
	if decision.SkillIntent.Prompt != "add widgets" {
		t.Fatalf("prompt = %q, want add widgets", decision.SkillIntent.Prompt)
	}
}

func TestClassifyDirectSkillActivationSplitsOnWhitespace(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:    "/alpha\tadd widgets",
		Surface: SurfaceWebWS,
	})
	if decision.Kind != DecisionSkillIntent {
		t.Fatalf("kind = %s, want %s: %#v", decision.Kind, DecisionSkillIntent, decision)
	}
	if len(decision.SkillIntent.Names) != 1 || decision.SkillIntent.Names[0] != "alpha" {
		t.Fatalf("names = %#v, want alpha", decision.SkillIntent.Names)
	}
	if decision.SkillIntent.Prompt != "add widgets" {
		t.Fatalf("prompt = %q, want add widgets", decision.SkillIntent.Prompt)
	}
}

func TestClassifyDirectSkillActivationAllowsEmptyPrompt(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:    "/flutter-adding-home-screen-widgets",
		Surface: SurfaceWebWS,
	})
	if decision.Kind != DecisionSkillIntent {
		t.Fatalf("kind = %s, want %s: %#v", decision.Kind, DecisionSkillIntent, decision)
	}
	if decision.SkillIntent.Prompt != "" {
		t.Fatalf("prompt = %q, want empty", decision.SkillIntent.Prompt)
	}
}

func TestClassifyChannelDirectSkillActivationWithBotSuffix(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:       "/alpha@Memoh do it",
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
	if len(decision.SkillIntent.Names) != 1 || decision.SkillIntent.Names[0] != "alpha" {
		t.Fatalf("names = %#v, want alpha", decision.SkillIntent.Names)
	}
	if decision.SkillIntent.Prompt != "do it" {
		t.Fatalf("prompt = %q", decision.SkillIntent.Prompt)
	}
}

func TestClassifyFixedCommandBeatsSkillSelector(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:    "/skill list",
		Surface: SurfaceWebWS,
		KnownCommand: func(resource string) bool {
			return resource == "skill"
		},
		WebActionSupported: func(resource, action string) bool {
			return resource == "skill" && action == "list"
		},
	})
	if decision.Kind != DecisionCommandAction {
		t.Fatalf("decision = %#v, want command action", decision)
	}
	if decision.Command.Resource != "skill" || decision.Command.Action != "list" {
		t.Fatalf("command = %#v, want skill list", decision.Command)
	}
}

func TestClassifyLegacySkillUseSyntaxRejects(t *testing.T) {
	tests := []string{
		"/skill use alpha -- do it",
		"/skill@Memoh use alpha -- do it",
	}
	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			decision := Classify(ClassifyInput{
				Text:       text,
				Surface:    SurfaceChannel,
				Directed:   true,
				BotAliases: []string{"Memoh"},
			})
			if decision.Kind != DecisionReject || decision.Code != CodeInvalidSkillSlashSyntax {
				t.Fatalf("decision = %#v, want reject %s", decision, CodeInvalidSkillSlashSyntax)
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
	decision := Classify(ClassifyInput{Text: "/wat?", Surface: SurfaceWebWS})
	if decision.Kind != DecisionUnknownSlash || decision.Code != CodeUnknownSlash {
		t.Fatalf("decision = %#v, want unknown slash", decision)
	}
}

// TestClassifyKnownCommandIgnoresAttachments pins the fail-closed scope:
// fixed commands never consume attachments, so a photo captioned "/status" —
// or an inline-keyboard tap whose synthetic message carries a reply ref the
// adapter can't vouch for — still classifies as a command action.
func TestClassifyKnownCommandIgnoresAttachments(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:           "/status",
		Surface:        SurfaceChannel,
		Directed:       true,
		HasAttachments: true,
		KnownCommand:   func(resource string) bool { return resource == "status" },
	})
	if decision.Kind != DecisionCommandAction || decision.Command.Resource != "status" {
		t.Fatalf("decision = %#v, want command action for /status with attachments", decision)
	}
}

// TestClassifyModePrefixWithAttachmentsStaysNormalChat: "/now" + photo is a
// normal chat message in now-mode, not a rejected control message.
func TestClassifyModePrefixWithAttachmentsStaysNormalChat(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:           "/now look at this",
		Surface:        SurfaceChannel,
		Directed:       true,
		SupportsMode:   true,
		HasAttachments: true,
	})
	if decision.Kind != DecisionNormalChat {
		t.Fatalf("decision = %#v, want normal chat for mode prefix with attachments", decision)
	}
}

// TestClassifySkillIntentRejectsAttachments pins the one place the attachment
// rule must hold: skill activation may not smuggle attachments (or unproven
// reply/forward attachments) into the requested-skill model context.
func TestClassifySkillIntentRejectsAttachments(t *testing.T) {
	decision := Classify(ClassifyInput{
		Text:           "/alpha do the thing",
		Surface:        SurfaceChannel,
		Directed:       true,
		HasAttachments: true,
	})
	if decision.Kind != DecisionReject || decision.Code != CodeSlashAttachmentsUnsupported {
		t.Fatalf("decision = %#v, want attachment reject for skill intent", decision)
	}
}
