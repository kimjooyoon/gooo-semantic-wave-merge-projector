package projector

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

func Generate(sourcePath, casesPath, outputPath, repositoryRoot string) (GenerationResult, error) {
	return GenerateWithOptions(sourcePath, casesPath, outputPath, repositoryRoot, ReviewOptions{})
}

func GenerateWithOptions(sourcePath, casesPath, outputPath, repositoryRoot string, review ReviewOptions) (GenerationResult, error) {
	if err := EnsureCallerDirectory(outputPath, repositoryRoot); err != nil {
		return GenerationResult{}, err
	}
	ir, _, err := LoadGraph(sourcePath)
	if err != nil {
		return GenerationResult{}, err
	}
	sourceAbsolute, err := filepath.Abs(sourcePath)
	if err != nil {
		return GenerationResult{}, err
	}
	reviewFixturePath := filepath.Join(filepath.Dir(filepath.Dir(sourceAbsolute)), ir.Graph.ReviewFixture)
	if _, err := LoadReviewFixture(reviewFixturePath, ir.Graph); err != nil {
		return GenerationResult{}, err
	}
	fixtures, err := LoadCases(casesPath, ir.Graph)
	if err != nil {
		return GenerationResult{}, err
	}
	normalResults := EvaluateFixtures(ir, fixtures)
	perturbedFixtures := append([]FixtureInput(nil), fixtures...)
	reverseFixtures(perturbedFixtures)
	perturbedResults := EvaluateFixtures(ir, perturbedFixtures)
	normalDigest := projectionDigest(normalResults)
	perturbedDigest := projectionDigest(perturbedResults)
	provenance := buildOperatorProvenance(ir.Graph, review)
	replay := ReplayReceipt{
		Schema:                   ReplaySchema,
		SourceDigest:             ir.SourceDigest,
		NormalInputOrder:         fixtureOrder(fixtures),
		OrderPerturbedInputOrder: fixtureOrder(perturbedFixtures),
		NormalDigest:             normalDigest,
		OrderPerturbedDigest:     perturbedDigest,
		Match:                    normalDigest == perturbedDigest,
		State:                    StateClosed,
		Reason:                   "ORDER_PERTURBED_REPLAY_MATCH",
		Immutable:                true,
		OperatorProvenance:       provenance,
	}
	if !replay.Match {
		replay.State = StateRefuted
		replay.Reason = ir.Graph.Rules["replay_mismatch"].Reason
	}
	result := GenerationResult{
		Denominator:  buildDenominator(ir, normalResults, provenance),
		Distribution: buildDistribution(ir, normalResults),
		Assertions:   buildAssertions(ir, normalResults, replay, provenance),
		Events:       buildEvents(normalResults),
		Replay:       replay,
		Report:       renderReport(ir, normalResults, replay, provenance),
	}
	if err := writeGeneration(outputPath, result); err != nil {
		return GenerationResult{}, err
	}
	return result, nil
}

func EnsureCallerDirectory(path, repositoryRoot string) error {
	if !filepath.IsAbs(path) {
		return errors.New("output directory must be an absolute caller-owned path")
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return err
	}
	output, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, output)
	if err != nil {
		return err
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return errors.New("output directory must be outside the source repository")
	}
	info, err := os.Stat(output)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("output path must be a directory")
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("caller-owned output directory must be empty")
	}
	return nil
}

func writeGeneration(outputPath string, result GenerationResult) error {
	outputs, err := outputBytes(result)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outputPath, 0o755); err != nil {
		return err
	}
	for _, name := range RequiredArtifactNames {
		if err := os.WriteFile(filepath.Join(outputPath, name), outputs[name], 0o644); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(outputPath)
	if err != nil {
		return err
	}
	if len(entries) != len(RequiredArtifactNames) {
		return errors.New("generation output does not contain exactly six artifacts")
	}
	return nil
}

func outputBytes(result GenerationResult) (map[string][]byte, error) {
	denominator, err := jsonWithNewline(result.Denominator)
	if err != nil {
		return nil, err
	}
	distribution, err := jsonWithNewline(result.Distribution)
	if err != nil {
		return nil, err
	}
	assertions, err := jsonWithNewline(struct {
		Schema     string               `json:"schema"`
		Assertions []GeneratedAssertion `json:"assertions"`
	}{Schema: AssertionSchema, Assertions: result.Assertions})
	if err != nil {
		return nil, err
	}
	replay, err := jsonWithNewline(result.Replay)
	if err != nil {
		return nil, err
	}
	events := bytes.Buffer{}
	for _, event := range result.Events {
		raw, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		events.Write(raw)
		events.WriteByte('\n')
	}
	projection, err := jsonWithNewline(result.Denominator)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		"wave-projection.json":      projection,
		"wave-distribution.json":    distribution,
		"generated-assertions.json": assertions,
		"projection-events.ndjson":  events.Bytes(),
		"replay-receipt.json":       replay,
		"report.md":                 []byte(result.Report),
	}, nil
}

func jsonWithNewline(value any) ([]byte, error) {
	raw, err := JSON(value)
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func buildDenominator(ir SemanticIR, results []CaseResult, provenance OperatorProvenanceReceipt) SemanticDenominator {
	return SemanticDenominator{
		Schema:              ProjectionSchema,
		IRSchema:            ir.Schema,
		SourcePath:          ir.SourcePath,
		SourceDigest:        ir.SourceDigest,
		GraphID:             ir.Graph.GraphID,
		Release:             ir.Graph.Release,
		ScenarioDenominator: len(ir.Graph.Cases),
		StateCounts:         stateCounts(results),
		ExpectedStateCounts: expectedStateCounts(ir.Graph.Cases),
		Precedence:          append([]string(nil), ir.Graph.Precedence...),
		ProofVector:         vectorCounts(ir.Graph, true),
		IndicatorVector:     vectorCounts(ir.Graph, false),
		Invariants:          append([]InvariantDecl(nil), ir.Graph.Invariants...),
		Fields:              append([]FieldDecl(nil), ir.Graph.Fields...),
		Authority: map[string]any{
			"semantic_graph":               "RELEASED_GOOO",
			"go_role":                      []string{"PARSER", "EVALUATOR", "GENERATOR", "RUNTIME"},
			"repository_writes":            ir.Graph.RepositoryWrites,
			"local_test_executions":        ir.Graph.LocalTestExecutions,
			"cross_project_required_gates": ir.Graph.CrossProjectRequiredGates,
			"caller_owned_output":          true,
		},
		OperatorProvenance: provenance,
		OutputArtifacts:    append([]string(nil), RequiredArtifactNames...),
		Cases:              results,
	}
}

func buildDistribution(ir SemanticIR, results []CaseResult) SemanticDistribution {
	return SemanticDistribution{
		Schema:           DistributionSchema,
		SourceDigest:     ir.SourceDigest,
		States:           stateCounts(results),
		ProofVector:      vectorCounts(ir.Graph, true),
		IndicatorVector:  vectorCounts(ir.Graph, false),
		DirectCountsOnly: true,
	}
}

func vectorCounts(graph SemanticGraph, proof bool) []VectorEntry {
	labels := []string{"FOUNDATION", "COHERENCE", "REGRESSION"}
	if !proof {
		labels = []string{"DRIVER", "OUTCOME", "GUARDRAIL"}
	}
	result := make([]VectorEntry, 0, len(labels))
	for _, label := range labels {
		count := 0
		for _, invariant := range graph.Invariants {
			value := invariant.Indicator
			if proof {
				value = invariant.Proof
			}
			if value == label {
				count++
			}
		}
		result = append(result, VectorEntry{Label: label, Count: count})
	}
	return result
}

func expectedStateCounts(cases []CaseContract) StateCounts {
	counts := StateCounts{Total: len(cases)}
	for _, value := range cases {
		switch value.Expected {
		case StateClosed:
			counts.Closed++
		case StateUnknown:
			counts.Unknown++
		case StateRefuted:
			counts.Refuted++
		}
	}
	return counts
}

func stateCounts(results []CaseResult) StateCounts {
	counts := StateCounts{Total: len(results)}
	for _, value := range results {
		switch value.State {
		case StateClosed:
			counts.Closed++
		case StateUnknown:
			counts.Unknown++
		case StateRefuted:
			counts.Refuted++
		}
	}
	return counts
}

func buildAssertions(ir SemanticIR, results []CaseResult, replay ReplayReceipt, provenance OperatorProvenanceReceipt) []GeneratedAssertion {
	assertions := []GeneratedAssertion{}
	for _, result := range results {
		assertions = append(assertions, GeneratedAssertion{
			Name:     "case/" + result.CaseID + "/expected-state",
			Expected: result.ExpectedState,
			Observed: result.State,
			Pass:     result.ExpectedState == result.State,
		})
		if result.State == StateUnknown {
			assertions = append(assertions, GeneratedAssertion{
				Name:     "case/" + result.CaseID + "/unknown-fields",
				Expected: "six-fields",
				Observed: result.Unknown != nil && result.Unknown.Valid(),
				Pass:     result.Unknown != nil && result.Unknown.Valid(),
			})
		}
	}
	counts := stateCounts(results)
	assertions = append(assertions,
		GeneratedAssertion{Name: "denominator/exact-count", Expected: 12, Observed: len(ir.Graph.Invariants), Pass: len(ir.Graph.Invariants) == 12},
		GeneratedAssertion{Name: "denominator/state-distribution", Expected: StateCounts{Total: 12, Closed: 4, Unknown: 4, Refuted: 4}, Observed: counts, Pass: counts == (StateCounts{Total: 12, Closed: 4, Unknown: 4, Refuted: 4})},
		GeneratedAssertion{Name: "denominator/proof-vector", Expected: vectorCounts(ir.Graph, true), Observed: vectorCounts(ir.Graph, true), Pass: vectorHasFour(vectorCounts(ir.Graph, true))},
		GeneratedAssertion{Name: "denominator/indicator-vector", Expected: vectorCounts(ir.Graph, false), Observed: vectorCounts(ir.Graph, false), Pass: vectorHasFour(vectorCounts(ir.Graph, false))},
		GeneratedAssertion{Name: "runtime/repository-writes", Expected: 0, Observed: ir.Graph.RepositoryWrites, Pass: ir.Graph.RepositoryWrites == 0},
		GeneratedAssertion{Name: "runtime/local-test-executions", Expected: 0, Observed: ir.Graph.LocalTestExecutions, Pass: ir.Graph.LocalTestExecutions == 0},
		GeneratedAssertion{Name: "runtime/cross-project-required-gates", Expected: 0, Observed: ir.Graph.CrossProjectRequiredGates, Pass: ir.Graph.CrossProjectRequiredGates == 0},
		GeneratedAssertion{Name: "output/exact-artifact-count", Expected: 6, Observed: len(RequiredArtifactNames), Pass: len(RequiredArtifactNames) == 6},
		GeneratedAssertion{Name: "replay/order-perturbed-match", Expected: true, Observed: replay.Match, Pass: replay.Match},
		GeneratedAssertion{Name: "operations/bootstrap-refutation-preserved", Expected: StateRefuted, Observed: provenance.BootstrapState, Pass: provenance.BootstrapState == StateRefuted},
		GeneratedAssertion{Name: "operations/pr-reviewed-gate", Expected: "PR_REVIEWED_OR_PR_REVIEWED_AND_MERGED", Observed: provenance.ReviewGate, Pass: provenance.ReviewGate == ir.Graph.ReviewGate.ReviewedState || provenance.ReviewGate == ir.Graph.ReviewGate.MergedState},
	)
	return assertions
}

func buildOperatorProvenance(graph SemanticGraph, review ReviewOptions) OperatorProvenanceReceipt {
	receipt := OperatorProvenanceReceipt{
		BootstrapState:    graph.BootstrapProvenance.State,
		BootstrapReason:   graph.BootstrapProvenance.Reason,
		BootstrapCommit:   graph.BootstrapProvenance.Commit,
		BootstrapRef:      graph.BootstrapProvenance.Ref,
		ReviewGate:        graph.ReviewGate.MissingState,
		PullRequestNumber: review.PullRequestNumber,
		MergeSHA:          review.MergeSHA,
		ReleaseTag:        review.ReleaseTag,
		Evidence:          []string{},
	}
	if review.PullRequestNumber <= 0 {
		receipt.Unknown = &UnknownClaim{
			Stage:         graph.BootstrapProvenance.Ref,
			Step:          graph.ReviewGate.ID,
			Reason:        graph.ReviewGate.Reason,
			UnknownClass:  graph.ReviewGate.UnknownClass,
			NextOperation: graph.ReviewGate.NextOperation,
			BlockedBy:     []string{"pull_request_number"},
		}
		return receipt
	}
	receipt.Evidence = append(receipt.Evidence, fmt.Sprintf("pull_request_number=%d", review.PullRequestNumber))
	receipt.ReviewGate = graph.ReviewGate.ReviewedState
	if review.MergeSHA != "" && review.ReleaseTag != "" {
		receipt.Evidence = append(receipt.Evidence, "merge_sha="+review.MergeSHA, "release_tag="+review.ReleaseTag, "review_event=annotated_release_tag")
		receipt.ReviewGate = graph.ReviewGate.MergedState
	}
	receipt.Evidence = sortedNonNil(receipt.Evidence)
	return receipt
}

func vectorHasFour(values []VectorEntry) bool {
	if len(values) != 3 {
		return false
	}
	for _, value := range values {
		if value.Count != 4 {
			return false
		}
	}
	return true
}

func buildEvents(results []CaseResult) []ProjectionEvent {
	events := make([]ProjectionEvent, 0, len(results))
	for _, result := range results {
		deferred := make([]string, 0, len(result.DeferredFrontier))
		for _, proposal := range result.DeferredFrontier {
			deferred = append(deferred, proposal.ProposalID)
		}
		events = append(events, ProjectionEvent{
			Schema: EventSchema, Ordinal: result.Ordinal, CaseID: result.CaseID,
			State: result.State, Result: result.Result,
			AcceptedWave: append([]string(nil), result.AcceptedWave...), Deferred: deferred,
		})
	}
	return events
}

func projectionDigest(results []CaseResult) string {
	canonical := make([]CaseResult, len(results))
	copy(canonical, results)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].Ordinal < canonical[j].Ordinal })
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "sha256:projection-encoding-error"
	}
	return DigestBytes(raw)
}

func fixtureOrder(fixtures []FixtureInput) []string {
	result := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		result = append(result, fixture.FixtureID)
	}
	return result
}

func reverseFixtures(fixtures []FixtureInput) {
	for left, right := 0, len(fixtures)-1; left < right; left, right = left+1, right-1 {
		fixtures[left], fixtures[right] = fixtures[right], fixtures[left]
	}
}

func reflectEqual(left, right any) bool {
	return reflect.DeepEqual(left, right)
}
