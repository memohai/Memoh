package botbackup

import (
	"context"

	"github.com/memohai/memoh/internal/bots"
)

// Summary reports what a live bot would export: per-section item counts and a
// sample of item labels. It powers the export dialog so users see counts and
// details (and skip empty sections) before exporting. Unlike Export it does not
// pause the bot or stream the workspace.
func (s *Service) Summary(ctx context.Context, botID string) (SummaryResult, error) {
	data, _, err := s.collect(ctx, botID, NormalizeExportOptions(ExportOptions{}))
	if err != nil {
		return SummaryResult{}, err
	}
	res := SummaryResult{Sections: []SectionSummary{}}
	if prof, err := roundTripJSON[bots.Bot](data.Profile); err == nil {
		res.Profile = &ProfilePreview{
			DisplayName: prof.DisplayName,
			AvatarURL:   prof.AvatarURL,
			Timezone:    prof.Timezone,
			IsActive:    prof.IsActive,
		}
	}
	add := func(key Section, value any, labelKeys ...string) {
		raw, _ := marshalJSON(value)
		res.Sections = append(res.Sections, SectionSummary{
			Key:       key,
			Count:     jsonArrayLen(raw),
			Items:     jsonArrayLabels(raw, sectionItemLimit, labelKeys...),
			Sensitive: isSensitiveSection(key),
		})
	}
	// settings.json backs two cards: behavior settings + model config.
	settingsRaw, _ := marshalJSON(data.Settings)
	res.Sections = append(res.Sections, SectionSummary{
		Key:   SectionSettings,
		Count: 1,
		Items: settingsLabels(settingsRaw),
	})
	modelsRaw, _ := marshalJSON(data.Dependencies.Models)
	res.Sections = append(res.Sections, SectionSummary{
		Key:       SectionModels,
		Count:     jsonArrayLen(modelsRaw),
		Sensitive: true,
		Items:     jsonArrayLabels(modelsRaw, sectionItemLimit, "name", "model_id"),
	})
	add(SectionACL, data.ACLRules, "description", "subject_channel_type")
	add(SectionChannels, data.Channels, "channel_type")
	add(SectionMCP, data.MCP, "name")
	add(SectionSchedules, data.Schedules, "name")
	add(SectionEmail, data.EmailBindings, "email_address")
	// History count is messages; the detail lists session titles.
	historyMsgRaw, _ := marshalJSON(data.History.Messages)
	historySessRaw, _ := marshalJSON(data.History.Sessions)
	res.Sections = append(res.Sections, SectionSummary{
		Key:   SectionHistory,
		Count: jsonArrayLen(historyMsgRaw),
		Items: jsonArrayLabels(historySessRaw, sectionItemLimit, "title", "type"),
	})
	add(SectionAssets, data.History.Assets, "name")
	// The workspace file count isn't known without building the archive; mark it
	// available with an unknown count (-1) so the dialog still offers it.
	res.Sections = append(res.Sections, SectionSummary{Key: SectionWorkspace, Count: -1})
	return res, nil
}
