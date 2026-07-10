package compaction

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/contextfrag"
)

func TestDecodeArtifactCoverageAcceptsStrictAndLegacyEmptyCoverage(t *testing.T) {
	t.Parallel()

	strict := strictTestCoveredSource("row-1", 1)
	decoded, err := DecodeArtifactCoverage(mustMarshalCoverage(t, strict))
	if err != nil {
		t.Fatalf("DecodeArtifactCoverage(strict) error = %v", err)
	}
	if len(decoded) != 1 || decoded[0].Ref.ContentHash != strict.Ref.ContentHash {
		t.Fatalf("DecodeArtifactCoverage(strict) = %#v", decoded)
	}

	for _, raw := range [][]byte{nil, []byte(" "), []byte("null"), []byte("[]")} {
		decoded, err := DecodeArtifactCoverage(raw)
		if err != nil || len(decoded) != 0 {
			t.Fatalf("DecodeArtifactCoverage(%q) = %#v, %v; want empty coverage", raw, decoded, err)
		}
	}
}

func TestDecodeArtifactCoverageRejectsInvalidPersistedCoverage(t *testing.T) {
	t.Parallel()

	missingHashIdentity := strictTestCoveredSource("missing-hash-identity", 1)
	missingHashIdentity.Ref.HashAlgo = ""
	missingHashIdentity.Ref.HashScope = ""
	missingHashIdentity.Ref.ContentHash = ""

	emptyContentHash := strictTestCoveredSource("empty-content-hash", 1)
	emptyContentHash.Ref.ContentHash = " "

	canonicalScope := strictTestCoveredSource("canonical-scope", 1)
	canonicalScope.Ref.HashScope = contextfrag.HashScopeCanonicalFragment

	duplicate := strictTestCoveredSource("duplicate", 1)
	newer := strictTestCoveredSource("newer", 2)
	older := strictTestCoveredSource("older", 1)

	tests := []struct {
		name    string
		covered []CoveredSource
		wantErr string
	}{
		{name: "missing hash identity", covered: []CoveredSource{missingHashIdentity}, wantErr: "sha256"},
		{name: "empty content hash", covered: []CoveredSource{emptyContentHash}, wantErr: "content hash"},
		{name: "canonical hash scope", covered: []CoveredSource{canonicalScope}, wantErr: "source_payload"},
		{name: "duplicate stable key", covered: []CoveredSource{duplicate, duplicate}, wantErr: "duplicate"},
		{name: "decreasing creation time", covered: []CoveredSource{newer, older}, wantErr: "created_at_ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := DecodeArtifactCoverage(mustMarshalCoverage(t, tt.covered...))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("DecodeArtifactCoverage() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCoverageIncludesRequiresExactPersistedHash(t *testing.T) {
	t.Parallel()

	strict := strictTestCoveredSource("row-1", 1)
	hashless := strict
	hashless.Ref.HashAlgo = ""
	hashless.Ref.HashScope = ""
	hashless.Ref.ContentHash = ""

	if !coverageIncludes([]CoveredSource{strict}, []CoveredSource{strict}) {
		t.Fatal("identical strict coverage did not match")
	}
	if coverageIncludes([]CoveredSource{strict}, []CoveredSource{hashless}) {
		t.Fatal("hashless required coverage acted as a wildcard")
	}
	if coverageIncludes([]CoveredSource{hashless}, []CoveredSource{hashless}) {
		t.Fatal("hashless persisted coverage matched itself")
	}
}

func TestPersistedMalformedOrHashlessLineageCoverageIsQuarantined(t *testing.T) {
	t.Parallel()

	hashless := strictTestCoveredSource("hashless-row", 1)
	hashless.Ref.HashAlgo = ""
	hashless.Ref.HashScope = ""
	hashless.Ref.ContentHash = ""

	tests := []struct {
		name     string
		coverage []byte
	}{
		{name: "malformed", coverage: []byte("{")},
		{name: "hashless", coverage: mustMarshalCoverage(t, hashless)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			row := projectionRow(t, "00000000-0000-0000-0000-00000000da01")
			row.ParentIds = []pgtype.UUID{mustProjectionUUID(t, "00000000-0000-0000-0000-00000000da02")}
			row.Coverage = tt.coverage
			artifact, err := artifactFromDBRow(row)
			if err != nil {
				t.Fatalf("artifactFromDBRow() error = %v", err)
			}
			if !artifact.CoverageMalformed {
				t.Fatal("invalid persisted coverage was not marked malformed")
			}

			frontier := buildArtifactFrontier([]Artifact{artifact})
			if len(frontier.Artifacts) != 0 || !hasLineageIssue(frontier.Issues, LineageIssueMissingDerivedCoverage) {
				t.Fatalf("invalid lineage coverage was not quarantined: frontier=%#v issues=%#v", frontier.Artifacts, frontier.Issues)
			}
		})
	}
}

func strictTestCoveredSource(id string, createdAtMs int64) CoveredSource {
	return CoveredSource{
		Ref: contextfrag.ContextRef{
			Namespace:   "bot_history_message",
			ID:          id,
			Version:     1,
			HashAlgo:    contextfrag.HashAlgoSHA256,
			HashScope:   contextfrag.HashScopeSourcePayload,
			ContentHash: "hash-" + id,
			Schema:      contextfrag.SchemaContextRef,
			Durability:  contextfrag.RefDurable,
		},
		CreatedAtMs: createdAtMs,
	}
}

func mustMarshalCoverage(t *testing.T, covered ...CoveredSource) []byte {
	t.Helper()
	raw, err := json.Marshal(covered)
	if err != nil {
		t.Fatalf("marshal coverage: %v", err)
	}
	return raw
}
