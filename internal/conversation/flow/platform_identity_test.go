package flow

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func TestBuildPlatformIdentitiesXML(t *testing.T) {
	t.Parallel()

	configs := []channel.ChannelConfig{
		{
			ID:               "tg-1",
			ChannelType:      channel.ChannelTypeTelegram,
			ExternalIdentity: "12345",
			SelfIdentity: map[string]any{
				"user_id":  "12345",
				"username": "memoh_bot",
			},
		},
		{
			ID:               "discord-1",
			ChannelType:      channel.ChannelTypeDiscord,
			ExternalIdentity: "98765",
			SelfIdentity: map[string]any{
				"name":     "Memoh & Co",
				"username": "@memoh",
			},
		},
	}

	got := buildPlatformIdentitiesXML(configs)
	want := strings.Join([]string{
		`<identity channel="discord" name="Memoh &amp; Co" username="@memoh" external_identity="98765"/>`,
		`<identity channel="telegram" user_id="12345" username="@memoh_bot" external_identity="12345"/>`,
	}, "\n")
	if got != want {
		t.Fatalf("unexpected XML:\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestBuildPlatformIdentityLineNormalizesAttrs(t *testing.T) {
	t.Parallel()

	got := buildPlatformIdentityLine(channel.ChannelConfig{
		ChannelType: channel.ChannelTypeTelegram,
		SelfIdentity: map[string]any{
			"123id":        7,
			"display name": `Memoh <Bot>`,
			"username":     "memoh",
			"xml_name":     "reserved",
		},
	})

	want := `<identity channel="telegram" attr_123id="7" display_name="Memoh &lt;Bot&gt;" username="@memoh" attr_xml_name="reserved"/>`
	if got != want {
		t.Fatalf("unexpected identity line:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestBuildPlatformIdentitiesSectionSkipsEmptyConfigs(t *testing.T) {
	t.Parallel()

	got := buildPlatformIdentitiesSection([]channel.ChannelConfig{{
		ID:          "local-1",
		ChannelType: channel.ChannelTypeLocal,
	}})
	if got != "" {
		t.Fatalf("expected empty section, got %q", got)
	}
}
