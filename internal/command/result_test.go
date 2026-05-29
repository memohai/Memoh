package command

import (
	"strings"
	"testing"
)

func sampleRecords(n int) []listRecord {
	records := make([]listRecord, 0, n)
	for i := 0; i < n; i++ {
		records = append(records, listRecord{fields: []kv{
			{"Name", "item" + strings.Repeat("x", i%3)},
			{"Type", "t"},
		}})
	}
	return records
}

func TestBuildListResultPagination(t *testing.T) {
	records := sampleRecords(27)

	page0 := buildListResult("T", "mcp", "list", nil, records, 0, 12, "hint")
	if page0.Interactive == nil || page0.Interactive.List == nil {
		t.Fatal("expected interactive list view")
	}
	lv := page0.Interactive.List
	if lv.Total != 27 || lv.Page != 0 || len(lv.Items) != 12 {
		t.Errorf("page0: total=%d page=%d items=%d, want 27/0/12", lv.Total, lv.Page, len(lv.Items))
	}
	if !strings.Contains(page0.Text, "Showing 12 of 27 items.") {
		t.Errorf("page0 text missing suffix: %q", page0.Text)
	}
	if !strings.Contains(page0.Text, "hint") {
		t.Errorf("page0 text missing hint: %q", page0.Text)
	}

	last := buildListResult("T", "mcp", "list", nil, records, 2, 12, "hint")
	if last.Interactive.List.Page != 2 || len(last.Interactive.List.Items) != 3 {
		t.Errorf("last page: page=%d items=%d, want 2/3", last.Interactive.List.Page, len(last.Interactive.List.Items))
	}
	if !strings.Contains(last.Text, "Showing 3 of 27 items.") {
		t.Errorf("last page text wrong: %q", last.Text)
	}

	// Page beyond the end clamps to the last page.
	clamped := buildListResult("T", "mcp", "list", nil, records, 99, 12, "hint")
	if clamped.Interactive.List.Page != 2 {
		t.Errorf("clamped page = %d, want 2", clamped.Interactive.List.Page)
	}
}

func TestBuildListResultSinglePageNoSuffix(t *testing.T) {
	res := buildListResult("T", "mcp", "list", nil, sampleRecords(5), 0, 12, "hint")
	if strings.Contains(res.Text, "Showing") {
		t.Errorf("single page should have no 'Showing' suffix: %q", res.Text)
	}
	if res.Interactive.List.Total != 5 || len(res.Interactive.List.Items) != 5 {
		t.Errorf("single page: total=%d items=%d, want 5/5", res.Interactive.List.Total, len(res.Interactive.List.Items))
	}
}

func TestListItemFromRecord(t *testing.T) {
	item := listItemFromRecord(listRecord{
		fields:   []kv{{"Name", "Foo"}, {"Type", "bar"}, {"Empty", ""}},
		selected: true,
	})
	if item.Label != "Foo" {
		t.Errorf("Label = %q, want Foo", item.Label)
	}
	if item.Detail != "Type: bar" {
		t.Errorf("Detail = %q, want 'Type: bar'", item.Detail)
	}
	if !item.Selected {
		t.Error("Selected = false, want true")
	}
}
