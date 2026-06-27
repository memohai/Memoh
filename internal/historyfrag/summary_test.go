package historyfrag

import (
	"testing"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
)

func TestSummaryRecordIsTypedCoveredAndRenderingEquivalent(t *testing.T) {
	t.Parallel()

	covered := []contextfrag.ContextRef{
		{Namespace: "bot_history_message", ID: "row-1", Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable},
		{Namespace: "bot_history_message", ID: "row-2", Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable},
	}

	rec := SummaryRecord("compact-1", "condensed text", covered, contextfrag.Scope{BotID: "bot-1"})

	if rec.Kind != contextfrag.KindConversationSummary {
		t.Fatalf("kind = %q, want conversation_summary", rec.Kind)
	}
	if rec.Lifecycle != LifecycleActiveSummary || rec.SourceKind != SourceCompactionLog {
		t.Fatalf("lifecycle/source = %s/%s", rec.Lifecycle, rec.SourceKind)
	}
	if rec.Ref.Namespace != NamespaceCompactionLog || rec.Ref.ID != "compact-1" {
		t.Fatalf("summary ref = %#v", rec.Ref)
	}
	if rec.Coverage == nil {
		t.Fatal("summary record must carry coverage")
	}
	if rec.Budget.Overflow != contextfrag.OverflowKeep {
		t.Fatalf("summary budget overflow = %q, want keep", rec.Budget.Overflow)
	}
	if len(rec.Coverage.CoveredRefs) != 2 || !rec.Coverage.SummaryRef.EqualIdentity(rec.Ref) {
		t.Fatalf("coverage mismatch: %#v", rec.Coverage)
	}

	want := conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("<summary>\ncondensed text\n</summary>")}
	if rec.ModelMessage.Role != want.Role || string(rec.ModelMessage.Content) != string(want.Content) {
		t.Fatalf("summary model message changed: %#v", rec.ModelMessage)
	}
}

func TestSummaryRecordToFragCarriesKindAndCoverage(t *testing.T) {
	t.Parallel()

	covered := []contextfrag.ContextRef{{Namespace: "bot_history_message", ID: "row-1", Schema: contextfrag.SchemaContextRef, Durability: contextfrag.RefDurable}}
	rec := SummaryRecord("compact-1", "condensed", covered, contextfrag.Scope{BotID: "bot-1"})
	rec.Budget = contextfrag.BudgetPolicy{MaxTokens: 12, Overflow: contextfrag.OverflowDrop}

	frag := ToFrag(rec)
	if frag.Kind != contextfrag.KindConversationSummary || frag.Slot != contextfrag.SlotHistory {
		t.Fatalf("frag kind/slot = %s/%s", frag.Kind, frag.Slot)
	}
	if frag.Budget != rec.Budget {
		t.Fatalf("frag budget = %#v, want %#v", frag.Budget, rec.Budget)
	}
	if frag.Coverage == nil || len(frag.Coverage.CoveredRefs) != 1 || frag.Coverage.CoveredRefs[0].ID != "row-1" {
		t.Fatalf("frag coverage mismatch: %#v", frag.Coverage)
	}
	manifest := contextfrag.BuildManifest([]contextfrag.ContextFrag{frag})
	if len(manifest.CoverageTrace) != 1 {
		t.Fatalf("coverage trace not built: %#v", manifest.CoverageTrace)
	}
	if len(manifest.Items) != 1 || manifest.Items[0].Budget != rec.Budget {
		t.Fatalf("manifest item budget = %#v, want %#v", manifest.Items, rec.Budget)
	}
}
