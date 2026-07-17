package db

import (
	"os"
	"strings"
	"testing"
)

func TestPipelineQueriesCarryExplicitTeamScope(t *testing.T) {
	const team = "public.memoh_current_team_id()"
	tests := []struct {
		file     string
		query    string
		required map[string]int
	}{
		{"messages.sql", "CompletePendingHistoryDelivery", map[string]int{"WHERE team_id = " + team: 1}},
		{"messages.sql", "ListExternalMessagePositionsBySession", map[string]int{"WHERE m.team_id = " + team: 1}},
		{"messages.sql", "GetLatestActiveTurnResponseAtBySession", map[string]int{"WHERE m.team_id = " + team: 1}},
		{"messages.sql", "ListUncoveredTurnResponsesBySession", map[string]int{"WHERE m.team_id = " + team: 1}},
		{"messages.sql", "ListVisibleTurnResponsesByRequest", map[string]int{
			"WHERE request.team_id = " + team:      1,
			"WHERE response.team_id = " + team:     1,
			"WHERE next_request.team_id = " + team: 1,
		}},
		{"messages.sql", "ListMessageEventCursorsByIDs", map[string]int{
			"AND source_event.team_id = " + team: 1,
			"WHERE m.team_id = " + team:          1,
		}},
		{"session_events.sql", "ClaimSessionEventDelivery", map[string]int{"WHERE event.team_id = " + team: 1}},
		{"session_events.sql", "CompleteSessionEventDelivery", map[string]int{
			"WHERE event.team_id = " + team:    1,
			"WHERE history.team_id = " + team:  1,
			"WHERE response.team_id = " + team: 1,
		}},
		{"session_events.sql", "CompleteSessionEventDeliveryWithResponse", map[string]int{
			"WHERE event.team_id = " + team:    1,
			"WHERE history.team_id = " + team:  1,
			"WHERE response.team_id = " + team: 1,
		}},
		{"session_events.sql", "RenewSessionEventDelivery", map[string]int{"WHERE team_id = " + team: 1}},
		{"session_events.sql", "ReleaseSessionEventDelivery", map[string]int{"WHERE team_id = " + team: 1}},
		{"session_events.sql", "LockSessionEventDeliveryClaim", map[string]int{"WHERE event.team_id = " + team: 1}},
		{"session_events.sql", "IsSessionEventDeliveryCompleted", map[string]int{"WHERE team_id = " + team: 1}},
		{"session_events.sql", "GetSessionEventIDByIdentity", map[string]int{"WHERE event.team_id = " + team: 1}},
		{"session_events.sql", "GetSessionEventDeliveryState", map[string]int{
			"WHERE event.team_id = " + team:           1,
			"WHERE message.team_id = " + team:         1,
			"WHERE response.team_id = " + team:        2,
			"ON response.team_id = " + team:           1,
			"WHERE visible_history.team_id = " + team: 1,
			"WHERE next_request.team_id = " + team:    1,
		}},
		{"sessions.sql", "GetSessionDiscussEventCursorFloor", map[string]int{"WHERE team_id = " + team: 1}},
	}

	sources := make(map[string]string)
	for _, test := range tests {
		source, ok := sources[test.file]
		if !ok {
			data, err := os.ReadFile("../../db/postgres/queries/" + test.file)
			if err != nil {
				t.Fatalf("read %s: %v", test.file, err)
			}
			source = string(data)
			sources[test.file] = source
		}
		query := scopedNamedQuery(t, source, test.query)
		for fragment, want := range test.required {
			if got := strings.Count(query, fragment); got != want {
				t.Errorf("%s %q count = %d, want %d", test.query, fragment, got, want)
			}
		}
	}
}

func scopedNamedQuery(t *testing.T, source, name string) string {
	t.Helper()
	startMarker := "-- name: " + name + " "
	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("query %s not found", name)
	}
	rest := source[start+len(startMarker):]
	if end := strings.Index(rest, "\n-- name: "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}
