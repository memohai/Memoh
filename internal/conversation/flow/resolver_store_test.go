package flow

import (
	"testing"

	"github.com/memohai/memoh/internal/conversation"
)

func TestBuildInteractionMetadataIncludesForwardConversation(t *testing.T) {
	t.Parallel()

	meta := buildInteractionMetadata(conversation.ChatRequest{
		SourceReplyToMessageID:    "reply-1",
		ReplySender:               "Original Sender",
		ReplyPreview:              "quoted text",
		ForwardMessageID:          "forward-1",
		ForwardFromUserID:         "source-user",
		ForwardFromConversationID: "source-conversation",
		ForwardSender:             "Source Channel",
		ForwardDate:               1710000000,
	})

	reply, ok := meta["reply"].(map[string]any)
	if !ok || reply["message_id"] != "reply-1" || reply["sender"] != "Original Sender" || reply["preview"] != "quoted text" {
		t.Fatalf("unexpected reply metadata: %#v", meta["reply"])
	}
	forward, ok := meta["forward"].(map[string]any)
	if !ok {
		t.Fatalf("expected forward metadata: %#v", meta)
	}
	if forward["message_id"] != "forward-1" ||
		forward["from_user_id"] != "source-user" ||
		forward["from_conversation_id"] != "source-conversation" ||
		forward["sender"] != "Source Channel" ||
		forward["date"] != int64(1710000000) {
		t.Fatalf("unexpected forward metadata: %#v", forward)
	}
}

func TestBuildInteractionMetadataIncludesRequestedSkills(t *testing.T) {
	t.Parallel()

	meta := buildInteractionMetadata(conversation.ChatRequest{
		RequestedSkills: []conversation.RequestedSkillContext{
			{
				Name:           "writer",
				SourceKind:     "managed",
				OpaqueSourceID: "src-1",
				Content:        "raw content must not be persisted in metadata",
				ContentHash:    "hash-must-not-leak",
				Identity:       "managed:src-1:writer",
			},
			{
				Name:           "writer",
				SourceKind:     "managed",
				OpaqueSourceID: "src-1",
				Identity:       "managed:src-1:writer",
			},
			{
				Name:       "reviewer",
				SourceKind: "plugin",
			},
		},
	})

	raw, ok := meta["model_requested_skills"].([]map[string]any)
	if !ok {
		t.Fatalf("expected requested skill metadata: %#v", meta["model_requested_skills"])
	}
	if len(raw) != 2 {
		t.Fatalf("expected deduped requested skills, got %#v", raw)
	}
	if raw[0]["name"] != "writer" || raw[0]["source_kind"] != "managed" {
		t.Fatalf("unexpected first skill metadata: %#v", raw[0])
	}
	if _, ok := raw[0]["opaque_source_id"]; ok {
		t.Fatalf("requested skill metadata leaked opaque source id: %#v", raw[0])
	}
	if _, ok := raw[0]["content"]; ok {
		t.Fatalf("requested skill metadata leaked content: %#v", raw[0])
	}
	if _, ok := raw[0]["content_hash"]; ok {
		t.Fatalf("requested skill metadata leaked content_hash: %#v", raw[0])
	}
	if _, ok := raw[0]["ref"]; ok {
		t.Fatalf("requested skill metadata leaked ref: %#v", raw[0])
	}
	if raw[1]["name"] != "reviewer" || raw[1]["source_kind"] != "plugin" {
		t.Fatalf("unexpected second skill metadata: %#v", raw[1])
	}
}

func TestBuildInteractionMetadataIncludesPublicSkillActivation(t *testing.T) {
	t.Parallel()

	meta := buildInteractionMetadata(conversation.ChatRequest{
		UserMessageKind: conversation.UserMessageKindSkillActivation,
		SkillActivation: &conversation.SkillActivation{
			Prompt: "do it",
			Skills: []conversation.SkillActivationSkill{{
				Name:        "writer",
				DisplayName: "Writer",
				Description: "short safe description",
				SourceKind:  "managed",
				State:       "effective",
			}},
		},
		RequestedSkills: []conversation.RequestedSkillContext{{
			Name:           "writer",
			SourceKind:     "managed",
			OpaqueSourceID: "opaque-source",
			Content:        "raw content must not leak",
			ContentHash:    "hash-must-not-leak",
			Identity:       "writer|opaque-source|hash",
		}},
	})

	if meta["user_message_kind"] != conversation.UserMessageKindSkillActivation {
		t.Fatalf("user_message_kind = %#v", meta["user_message_kind"])
	}
	public, ok := meta["skill_activation"].(map[string]any)
	if !ok {
		t.Fatalf("expected public skill activation metadata: %#v", meta["skill_activation"])
	}
	if public["prompt"] != "do it" {
		t.Fatalf("prompt = %#v, want do it", public["prompt"])
	}
	skills, ok := public["skills"].([]map[string]any)
	if !ok || len(skills) != 1 {
		t.Fatalf("public skills = %#v, want one", public["skills"])
	}
	if skills[0]["name"] != "writer" || skills[0]["display_name"] != "Writer" {
		t.Fatalf("unexpected public skill: %#v", skills[0])
	}
	for _, key := range []string{"opaque_source_id", "content", "content_hash", "ref"} {
		if _, ok := skills[0][key]; ok {
			t.Fatalf("public skill leaked %s: %#v", key, skills[0])
		}
	}
	if _, ok := meta["audit_requested_skills"]; ok {
		t.Fatalf("audit metadata leaked into message metadata: %#v", meta["audit_requested_skills"])
	}
}
