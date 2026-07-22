package handlers

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func runtimeContractFixturePath(t *testing.T, filename string) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve runtime contract fixture path")
	}
	return filepath.Join(filepath.Dir(sourceFile), "..", "..", "apps", "web", "src", "store", "__fixtures__", filename)
}

func assertRuntimeContractFixtureCurrent(t *testing.T, fixture any, filename string) {
	t.Helper()
	got, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatalf("marshal runtime contract fixture: %v", err)
	}
	got = append(got, '\n')
	path := runtimeContractFixturePath(t, filename)
	if os.Getenv(runtimeFixtureUpdateEnv) == "1" {
		// The committed fixture is public test data and should remain readable like other source files.
		//nolint:gosec
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("update runtime contract fixture: %v", err)
		}
	}
	// The path is derived from this source file and cannot be supplied externally.
	//nolint:gosec
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime contract fixture: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("runtime contract fixture %s drifted; review the wire change, then run mise run runtime-contract-fixtures\nwant:\n%s\ngot:\n%s", filename, want, got)
	}
}

func TestRichActiveRunContractFixtureIsCurrent(t *testing.T) {
	assertRuntimeContractFixtureCurrent(t, buildRichActiveRunContractFixture(t), "runtime-rich-active-run.contract.json")
}

func TestInterruptedRunContractFixtureIsCurrent(t *testing.T) {
	assertRuntimeContractFixtureCurrent(t, buildInterruptedRunContractFixture(t), "runtime-interrupted-run.contract.json")
}

func TestReplacementOperationContractFixtureIsCurrent(t *testing.T) {
	assertRuntimeContractFixtureCurrent(t, buildRuntimeReplacementContractFixture(t), "runtime-replacement-operations.contract.json")
}

func TestGenerationReuseContractFixtureIsCurrent(t *testing.T) {
	assertRuntimeContractFixtureCurrent(t, buildRuntimeGenerationReuseContractFixture(t), "runtime-generation-reuse.contract.json")
}

func TestRuntimeRecoveryContractFixtureIsCurrent(t *testing.T) {
	assertRuntimeContractFixtureCurrent(t, buildRuntimeRecoveryContractFixture(t), "runtime-recovery.contract.json")
}
