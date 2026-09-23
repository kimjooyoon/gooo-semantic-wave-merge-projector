package projector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalProjection(t *testing.T) {
	root := repositoryRoot(t)
	source := filepath.Join(root, ".gooo", "semantic-wave-merge-projector.gooo")
	cases := filepath.Join(root, "fixtures", "cases")
	ir, _, err := LoadGraph(source)
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := LoadCases(cases, ir.Graph)
	if err != nil {
		t.Fatal(err)
	}
	results := EvaluateFixtures(ir, fixtures)
	if got := stateCounts(results); got != (StateCounts{Total: 12, Closed: 4, Unknown: 4, Refuted: 4}) {
		t.Fatalf("state counts = %+v", got)
	}
	if _, err := LoadReviewFixture(filepath.Join(root, "fixtures", "operational", "pr-reviewed-release.json"), ir.Graph); err != nil {
		t.Fatal(err)
	}
	if len(ir.Graph.HistoricalReleases) != 1 || ir.Graph.HistoricalReleases[0].Tag != "v0.1.0" || ir.Graph.HistoricalReleases[0].State != StateRefuted {
		t.Fatalf("historical release provenance = %+v", ir.Graph.HistoricalReleases)
	}
	for _, result := range results {
		if result.State != result.ExpectedState {
			t.Fatalf("case %s state = %s, expected %s", result.CaseID, result.State, result.ExpectedState)
		}
		if result.State == StateUnknown && !result.Unknown.Valid() {
			t.Fatalf("case %s has incomplete UNKNOWN claim", result.CaseID)
		}
	}
}

func TestGenerationUsesCallerOwnedOutput(t *testing.T) {
	root := repositoryRoot(t)
	output := filepath.Join(t.TempDir(), "wave-output")
	result, err := Generate(
		filepath.Join(root, ".gooo", "semantic-wave-merge-projector.gooo"),
		filepath.Join(root, "fixtures", "cases"), output, root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Replay.Match || !result.Replay.Immutable {
		t.Fatalf("replay receipt = %+v", result.Replay)
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(RequiredArtifactNames) {
		t.Fatalf("artifact count = %d", len(entries))
	}
}

func TestReadAfterWriteDependencyIsOrdered(t *testing.T) {
	root := repositoryRoot(t)
	ir, _, err := LoadGraph(filepath.Join(root, ".gooo", "semantic-wave-merge-projector.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := LoadCases(filepath.Join(root, "fixtures", "cases"), ir.Graph)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		if fixture.FixtureID != "read-after-write-dependency" {
			continue
		}
		result := EvaluateFixture(ir, fixture)
		if result.State != StateClosed || len(result.AcceptedWave) != 2 || result.AcceptedWave[0] != "proposal-raw-writer" || result.AcceptedWave[1] != "proposal-raw-reader" {
			t.Fatalf("read-after-write projection = %+v", result)
		}
		return
	}
	t.Fatal("read-after-write fixture not found")
}

func TestDuplicateProposalReplayOrderingIsCanonical(t *testing.T) {
	root := repositoryRoot(t)
	ir, _, err := LoadGraph(filepath.Join(root, ".gooo", "semantic-wave-merge-projector.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	mutable, verified := false, true
	proposal := func(read, write, evidence string) ProposalInput {
		return ProposalInput{
			ProposalID:         "duplicate-proposal",
			BaseLedgerDigest:   ir.Graph.LedgerDigest,
			SemanticReadSet:    []string{read},
			SemanticWriteSet:   []string{write},
			RequiredEvidence:   []string{evidence},
			ToolReleaseLocks:   []ToolReleaseLockInput{{ToolID: "gooo-evaluator", ReleaseDigest: "sha256:tool-v1", Mutable: &mutable, Verified: &verified}},
			CausalDependencies: []string{},
			AuthorityScope:     []string{"self-improvement.proposal.create"},
		}
	}
	fixture := FixtureInput{
		FixtureID:        "malformed-proposal",
		ExpectedState:    StateRefuted,
		BaseLedgerDigest: ir.Graph.LedgerDigest,
		EvidenceDigests:  []string{"evidence-a", "evidence-b"},
		Proposals: []ProposalInput{
			proposal("semantic.z", "semantic.a", "evidence-a"),
			proposal("semantic.a", "semantic.z", "evidence-b"),
		},
	}
	reversed := fixture
	reversed.Proposals = []ProposalInput{fixture.Proposals[1], fixture.Proposals[0]}
	first, second := EvaluateFixture(ir, fixture), EvaluateFixture(ir, reversed)
	if first.State != StateRefuted || second.State != StateRefuted {
		t.Fatalf("duplicate proposal states = %s, %s", first.State, second.State)
	}
	if got, want := projectionDigest([]CaseResult{second}), projectionDigest([]CaseResult{first}); got != want {
		t.Fatalf("duplicate proposal replay digest changed: got %s, want %s", got, want)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join(working, "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
