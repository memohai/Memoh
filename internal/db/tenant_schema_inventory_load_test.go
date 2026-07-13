package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// tablesInventory is the minimal shape of tables.json needed to assert coverage.
type tablesInventory struct {
	BaseCommit string `json:"base_commit"`
	TableCount int    `json:"table_count"`
	Tables     []struct {
		Name           string `json:"name"`
		Classification string `json:"classification"`
	} `json:"tables"`
	Views []struct {
		Name string `json:"name"`
	} `json:"views"`
}

// setNullEntry is one ON DELETE SET NULL FK row.
type setNullEntry struct {
	ChildTable  string `json:"child_table"`
	Column      string `json:"column"`
	ParentTable string `json:"parent_table"`
	Source      string `json:"source"`
	Migrated    bool   `json:"migrated"`
}

// setNullInventory is the minimal shape of set_null_fks.json.
type setNullInventory struct {
	BaseCommit           string         `json:"base_commit"`
	CanonicalCount       int            `json:"canonical_count"`
	HistoricalExtraCount int            `json:"historical_extra_count"`
	TotalDistinct        int            `json:"total_distinct"`
	Canonical            []setNullEntry `json:"canonical"`
	Historical           []setNullEntry `json:"historical"`
}

// TestInventoryCoversPostgresAndSetNull loads the pinned tables.json and
// set_null_fks.json inventory artifacts and asserts they cover the canonical
// PostgreSQL schema: a full table inventory (>= 48) and the exact canonical
// ON DELETE SET NULL set (47), matching the pinned oracle. Every SET NULL entry
// must carry non-empty child_table/column/parent_table/source.
func TestInventoryCoversPostgresAndSetNull(t *testing.T) {
	base := pinnedBaseCommit(t)
	dir := filepath.Join(inventoryRoot, base)

	// ---- tables.json ----
	rawTables, err := os.ReadFile(filepath.Join(dir, "tables.json"))
	if err != nil {
		t.Fatalf("read tables.json: %v", err)
	}
	var tables tablesInventory
	if err := json.Unmarshal(rawTables, &tables); err != nil {
		t.Fatalf("parse tables.json: %v", err)
	}
	if tables.BaseCommit != base {
		t.Fatalf("tables.json base_commit %q != pinned dir %q", tables.BaseCommit, base)
	}
	if tables.TableCount <= 0 {
		t.Fatalf("tables.json table_count must be > 0, got %d", tables.TableCount)
	}
	if tables.TableCount < 48 {
		t.Fatalf("tables.json table_count must be >= 48 (canonical 0001_init.up.sql has 48), got %d", tables.TableCount)
	}
	if len(tables.Tables) != tables.TableCount {
		t.Fatalf("tables.json table_count %d != len(tables) %d", tables.TableCount, len(tables.Tables))
	}
	seen := make(map[string]bool, len(tables.Tables))
	for i, tb := range tables.Tables {
		if tb.Name == "" {
			t.Fatalf("tables.json entry %d has empty name", i)
		}
		if tb.Classification == "" {
			t.Fatalf("tables.json table %q has empty classification", tb.Name)
		}
		if seen[tb.Name] {
			t.Fatalf("tables.json duplicate table name %q", tb.Name)
		}
		seen[tb.Name] = true
	}
	// The view must be recorded separately.
	if len(tables.Views) == 0 {
		t.Fatal("tables.json must record the bot_visible_history_messages view in the views array")
	}

	// ---- set_null_fks.json ----
	rawSN, err := os.ReadFile(filepath.Join(dir, "set_null_fks.json"))
	if err != nil {
		t.Fatalf("read set_null_fks.json: %v", err)
	}
	var sn setNullInventory
	if err := json.Unmarshal(rawSN, &sn); err != nil {
		t.Fatalf("parse set_null_fks.json: %v", err)
	}
	if sn.BaseCommit != base {
		t.Fatalf("set_null_fks.json base_commit %q != pinned dir %q", sn.BaseCommit, base)
	}

	// Canonical section must have exactly 47 entries and match the declared count.
	const wantCanonical = 47
	if len(sn.Canonical) != wantCanonical {
		t.Fatalf("canonical SET NULL section must have exactly %d entries, got %d", wantCanonical, len(sn.Canonical))
	}
	if sn.CanonicalCount != wantCanonical {
		t.Fatalf("set_null_fks.json canonical_count must be %d, got %d", wantCanonical, sn.CanonicalCount)
	}

	// placeholderTokens are SQL keywords that indicate a mis-parsed multi-line
	// ALTER TABLE ... ADD CONSTRAINT ... FOREIGN KEY row (the child_table/column
	// must be the real table/column, never a stray keyword).
	placeholderTokens := map[string]bool{"FOREIGN": true, "ALTER": true, "KEY": true, "CONSTRAINT": true, "REFERENCES": true, "TABLE": true, "ADD": true}

	// Every SET NULL entry (canonical + historical) must have non-empty required
	// fields and must not carry a mis-parsed placeholder keyword.
	checkEntries := func(section string, entries []setNullEntry) {
		for i, e := range entries {
			if e.ChildTable == "" {
				t.Fatalf("%s SET NULL entry %d has empty child_table", section, i)
			}
			if e.Column == "" {
				t.Fatalf("%s SET NULL entry %d (%s) has empty column", section, i, e.ChildTable)
			}
			if e.ParentTable == "" {
				t.Fatalf("%s SET NULL entry %d (%s) has empty parent_table", section, i, e.ChildTable)
			}
			if e.Source == "" {
				t.Fatalf("%s SET NULL entry %d (%s.%s) has empty source", section, i, e.ChildTable, e.Column)
			}
			for field, val := range map[string]string{"child_table": e.ChildTable, "column": e.Column, "parent_table": e.ParentTable} {
				if placeholderTokens[val] {
					t.Fatalf("%s SET NULL entry %d has placeholder token %q in %s (mis-parsed ALTER-based FK)", section, i, val, field)
				}
			}
		}
	}
	checkEntries("canonical", sn.Canonical)
	checkEntries("historical", sn.Historical)

	// Canonical entries must all be sourced as "canonical".
	for i, e := range sn.Canonical {
		if e.Source != "canonical" {
			t.Fatalf("canonical SET NULL entry %d (%s.%s) source must be \"canonical\", got %q", i, e.ChildTable, e.Column, e.Source)
		}
	}

	// total_distinct bookkeeping must be internally consistent.
	if sn.HistoricalExtraCount != len(sn.Historical) {
		t.Fatalf("set_null_fks.json historical_extra_count %d != len(historical) %d", sn.HistoricalExtraCount, len(sn.Historical))
	}
	if sn.TotalDistinct != sn.CanonicalCount+sn.HistoricalExtraCount {
		t.Fatalf("set_null_fks.json total_distinct %d != canonical %d + historical %d", sn.TotalDistinct, sn.CanonicalCount, sn.HistoricalExtraCount)
	}

	// Cross-check the canonical section against the pinned oracle: same set of
	// (child_table, column, parent_table) triples.
	rawOracle, err := os.ReadFile(filepath.Join(dir, ".setnull_canonical_oracle.json"))
	if err != nil {
		t.Fatalf("read .setnull_canonical_oracle.json: %v", err)
	}
	var oracle []struct {
		ChildTable  string `json:"child_table"`
		Column      string `json:"column"`
		ParentTable string `json:"parent_table"`
	}
	if err := json.Unmarshal(rawOracle, &oracle); err != nil {
		t.Fatalf("parse oracle: %v", err)
	}
	if len(oracle) != wantCanonical {
		t.Fatalf("oracle must have %d rows, got %d", wantCanonical, len(oracle))
	}
	type triple struct{ c, col, p string }
	oracleSet := make(map[triple]bool, len(oracle))
	for _, o := range oracle {
		oracleSet[triple{o.ChildTable, o.Column, o.ParentTable}] = true
	}
	for i, e := range sn.Canonical {
		tr := triple{e.ChildTable, e.Column, e.ParentTable}
		if !oracleSet[tr] {
			t.Fatalf("canonical SET NULL entry %d %+v not present in pinned oracle", i, tr)
		}
	}
	if len(oracleSet) != len(sn.Canonical) {
		t.Fatalf("canonical SET NULL section has %d distinct triples but oracle has %d", len(sn.Canonical), len(oracleSet))
	}
}
