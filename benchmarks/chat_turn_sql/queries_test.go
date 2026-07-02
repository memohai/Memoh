package main

import (
	"strings"
	"testing"
)

func TestLoadQueriesLoadsAllKnownPostgresTemplates(t *testing.T) {
	queries, err := loadQueries("queries/postgres")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range knownQueries {
		if queries[name] == "" {
			t.Fatalf("missing query %s", name)
		}
	}
}

func TestQueryDefinitionsHaveProductionSource(t *testing.T) {
	if len(queryDefinitions) != len(knownQueries) {
		t.Fatalf("definitions=%d names=%d", len(queryDefinitions), len(knownQueries))
	}
	seen := map[string]bool{}
	for _, def := range queryDefinitions {
		if def.Name == "" || def.SourceFile == "" || def.SourceName == "" {
			t.Fatalf("incomplete query definition: %#v", def)
		}
		if seen[def.Name] {
			t.Fatalf("duplicate query definition %s", def.Name)
		}
		seen[def.Name] = true
		if !strings.HasPrefix(def.SourceFile, "db/postgres/queries/") {
			t.Fatalf("unexpected source file for %s: %s", def.Name, def.SourceFile)
		}
		if len(def.Args) == 0 {
			t.Fatalf("missing args for %s", def.Name)
		}
	}
}
