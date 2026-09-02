package projector

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func LoadGraph(path string) (SemanticIR, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return SemanticIR{}, nil, err
	}
	graph, err := parseGraph(path, raw)
	if err != nil {
		return SemanticIR{}, nil, err
	}
	return SemanticIR{
		Schema:       IRSchema,
		SourcePath:   filepath.ToSlash(path),
		SourceDigest: DigestBytes(raw),
		Graph:        graph,
	}, raw, nil
}

func parseGraph(path string, raw []byte) (SemanticGraph, error) {
	graph := SemanticGraph{
		Schema:      GraphSchema,
		Artifacts:   []ArtifactDecl{},
		Fields:      []FieldDecl{},
		States:      map[string]StateDecl{},
		Authorities: []AuthorityDecl{},
		Invariants:  []InvariantDecl{},
		Rules:       map[string]RuleDecl{},
		Cases:       []CaseContract{},
		HistoricalReleases: []HistoricalReleaseDecl{},
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	lineNumber := 0
	seenKinds := map[string]bool{}
	seenIDs := map[string]string{}
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "package" || fields[0] == "namespace" {
			if len(fields) != 2 || fields[1] == "" {
				return SemanticGraph{}, fmt.Errorf("line %d: invalid %s declaration", lineNumber, fields[0])
			}
			continue
		}
		values, err := keyValues(fields[1:])
		if err != nil {
			return SemanticGraph{}, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		location := SourceLocation{Path: filepath.ToSlash(path), Line: lineNumber, Column: 1}
		switch fields[0] {
		case "graph":
			if seenKinds["graph"] || values["id"] == "" || values["release"] == "" {
				return SemanticGraph{}, fmt.Errorf("line %d: invalid graph declaration", lineNumber)
			}
			precedence := splitList(values["precedence"])
			repositoryWrites, err := integerValue(values, "repository_writes")
			if err != nil {
				return SemanticGraph{}, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			localTests, err := integerValue(values, "local_test_executions")
			if err != nil {
				return SemanticGraph{}, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			crossProjectGates, err := integerValue(values, "cross_project_required_gates")
			if err != nil {
				return SemanticGraph{}, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			artifactCount, err := integerValue(values, "output_artifact_count")
			if err != nil {
				return SemanticGraph{}, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			graph.GraphID = values["id"]
			graph.Release = values["release"]
			graph.Precedence = precedence
			graph.RepositoryWrites = repositoryWrites
			graph.LocalTestExecutions = localTests
			graph.CrossProjectRequiredGates = crossProjectGates
			graph.OutputArtifactCount = artifactCount
			graph.LedgerDigest = values["ledger_digest"]
			seenKinds["graph"] = true
		case "artifact":
			ordinal, err := integerValue(values, "ordinal")
			if err != nil || ordinal < 1 || values["name"] == "" {
				return SemanticGraph{}, fmt.Errorf("line %d: invalid artifact declaration", lineNumber)
			}
			graph.Artifacts = append(graph.Artifacts, ArtifactDecl{Ordinal: ordinal, Name: values["name"], Source: location})
		case "field":
			field, err := parseField(values, location, lineNumber)
			if err != nil {
				return SemanticGraph{}, err
			}
			if err := registerID(seenIDs, field.Name, "field", lineNumber); err != nil {
				return SemanticGraph{}, err
			}
			graph.Fields = append(graph.Fields, field)
		case "state":
			state, err := parseState(values, location, lineNumber)
			if err != nil {
				return SemanticGraph{}, err
			}
			if _, exists := graph.States[state.Name]; exists {
				return SemanticGraph{}, fmt.Errorf("line %d: duplicate state %s", lineNumber, state.Name)
			}
			graph.States[state.Name] = state
		case "authority":
			authority, err := parseAuthority(values, location, lineNumber)
			if err != nil {
				return SemanticGraph{}, err
			}
			if err := registerID(seenIDs, authority.Name, "authority", lineNumber); err != nil {
				return SemanticGraph{}, err
			}
			graph.Authorities = append(graph.Authorities, authority)
		case "provenance":
			provenance, err := parseProvenance(values, location, lineNumber)
			if err != nil {
				return SemanticGraph{}, err
			}
			if graph.BootstrapProvenance.ID != "" {
				return SemanticGraph{}, fmt.Errorf("line %d: duplicate bootstrap provenance", lineNumber)
			}
			graph.BootstrapProvenance = provenance
		case "review_gate":
			gate, err := parseReviewGate(values, location, lineNumber)
			if err != nil {
				return SemanticGraph{}, err
			}
			if graph.ReviewGate.ID != "" {
				return SemanticGraph{}, fmt.Errorf("line %d: duplicate review gate", lineNumber)
			}
			graph.ReviewGate = gate
		case "review_fixture":
			if graph.ReviewFixture != "" || values["path"] == "" {
				return SemanticGraph{}, fmt.Errorf("line %d: invalid review fixture declaration", lineNumber)
			}
			graph.ReviewFixture = values["path"]
		case "historical_release":
			release, err := parseHistoricalRelease(values, location, lineNumber)
			if err != nil {
				return SemanticGraph{}, err
			}
			graph.HistoricalReleases = append(graph.HistoricalReleases, release)
		case "invariant":
			invariant, err := parseInvariant(values, location, lineNumber)
			if err != nil {
				return SemanticGraph{}, err
			}
			if err := registerID(seenIDs, invariant.StableID, "invariant", lineNumber); err != nil {
				return SemanticGraph{}, err
			}
			graph.Invariants = append(graph.Invariants, invariant)
		case "rule":
			rule, err := parseRule(values, location, lineNumber)
			if err != nil {
				return SemanticGraph{}, err
			}
			if _, exists := graph.Rules[rule.StableID]; exists {
				return SemanticGraph{}, fmt.Errorf("line %d: duplicate rule %s", lineNumber, rule.StableID)
			}
			graph.Rules[rule.StableID] = rule
		case "case":
			caseContract, err := parseCase(values, location, lineNumber)
			if err != nil {
				return SemanticGraph{}, err
			}
			if err := registerID(seenIDs, caseContract.StableID, "case", lineNumber); err != nil {
				return SemanticGraph{}, err
			}
			graph.Cases = append(graph.Cases, caseContract)
		default:
			return SemanticGraph{}, fmt.Errorf("line %d: unsupported declaration %s", lineNumber, fields[0])
		}
	}
	if err := scanner.Err(); err != nil {
		return SemanticGraph{}, err
	}
	if err := validateGraph(graph); err != nil {
		return SemanticGraph{}, err
	}
	return graph, nil
}

func parseField(values map[string]string, location SourceLocation, line int) (FieldDecl, error) {
	ordinal, err := integerValue(values, "ordinal")
	if err != nil || ordinal < 1 || values["name"] == "" || values["type"] == "" {
		return FieldDecl{}, fmt.Errorf("line %d: incomplete field", line)
	}
	required, err := boolValue(values, "required")
	if err != nil {
		return FieldDecl{}, fmt.Errorf("line %d: %w", line, err)
	}
	return FieldDecl{Ordinal: ordinal, Name: values["name"], Type: values["type"], Required: required, Source: location}, nil
}

func parseState(values map[string]string, location SourceLocation, line int) (StateDecl, error) {
	ordinal, err := integerValue(values, "ordinal")
	if err != nil || ordinal < 1 || values["name"] == "" || values["result"] == "" || values["next_operation"] == "" {
		return StateDecl{}, fmt.Errorf("line %d: incomplete state", line)
	}
	return StateDecl{Ordinal: ordinal, Name: values["name"], Result: values["result"], NextOperation: values["next_operation"], Source: location}, nil
}

func parseAuthority(values map[string]string, location SourceLocation, line int) (AuthorityDecl, error) {
	ordinal, err := integerValue(values, "ordinal")
	if err != nil || ordinal < 1 || values["name"] == "" {
		return AuthorityDecl{}, fmt.Errorf("line %d: incomplete authority", line)
	}
	scope := splitList(values["scope"])
	if len(scope) == 0 {
		return AuthorityDecl{}, fmt.Errorf("line %d: authority scope is required", line)
	}
	return AuthorityDecl{Ordinal: ordinal, Name: values["name"], Scope: scope, Source: location}, nil
}

func parseProvenance(values map[string]string, location SourceLocation, line int) (BootstrapProvenanceDecl, error) {
	provenance := BootstrapProvenanceDecl{ID: values["id"], State: values["state"], Reason: values["reason"], Commit: values["commit"], Ref: values["ref"], Source: location}
	if provenance.ID == "" || provenance.State == "" || provenance.Reason == "" || provenance.Commit == "" || provenance.Ref == "" {
		return BootstrapProvenanceDecl{}, fmt.Errorf("line %d: incomplete bootstrap provenance", line)
	}
	return provenance, nil
}

func parseReviewGate(values map[string]string, location SourceLocation, line int) (ReviewGateDecl, error) {
	required, err := boolValue(values, "required")
	if err != nil {
		return ReviewGateDecl{}, fmt.Errorf("line %d: %w", line, err)
	}
	gate := ReviewGateDecl{ID: values["id"], Required: required, MissingState: values["missing_state"], ReviewedState: values["reviewed_state"], MergedState: values["merged_state"], Reason: values["reason"], UnknownClass: values["unknown_class"], NextOperation: values["next_operation"], Source: location}
	if gate.ID == "" || gate.MissingState == "" || gate.ReviewedState == "" || gate.MergedState == "" || gate.Reason == "" || gate.UnknownClass == "" || gate.NextOperation == "" {
		return ReviewGateDecl{}, fmt.Errorf("line %d: incomplete review gate", line)
	}
	return gate, nil
}

func parseHistoricalRelease(values map[string]string, location SourceLocation, line int) (HistoricalReleaseDecl, error) {
	ordinal, err := integerValue(values, "ordinal")
	if err != nil || ordinal < 1 || values["tag"] == "" || values["run"] == "" || values["state"] == "" || values["reason"] == "" {
		return HistoricalReleaseDecl{}, fmt.Errorf("line %d: incomplete historical release", line)
	}
	return HistoricalReleaseDecl{Ordinal: ordinal, Tag: values["tag"], Run: values["run"], State: values["state"], Reason: values["reason"], Source: location}, nil
}

func parseInvariant(values map[string]string, location SourceLocation, line int) (InvariantDecl, error) {
	ordinal, err := integerValue(values, "ordinal")
	if err != nil || ordinal < 1 {
		return InvariantDecl{}, fmt.Errorf("line %d: invalid invariant ordinal", line)
	}
	invariant := InvariantDecl{
		Ordinal: ordinal, StableID: values["id"], Class: values["class"], Proof: values["proof"],
		Indicator: values["indicator"], Stage: values["stage"], Step: values["step"],
		DependsOn: splitList(values["depends_on"]), Source: location,
	}
	if invariant.StableID == "" || invariant.Class == "" || invariant.Proof == "" || invariant.Indicator == "" || invariant.Stage == "" || invariant.Step == "" {
		return InvariantDecl{}, fmt.Errorf("line %d: incomplete invariant", line)
	}
	return invariant, nil
}

func parseRule(values map[string]string, location SourceLocation, line int) (RuleDecl, error) {
	rule := RuleDecl{
		StableID: values["id"], Invariant: values["invariant"], State: values["state"], Reason: values["reason"],
		UnknownClass: values["unknown_class"], NextOperation: values["next_operation"], Source: location,
	}
	if rule.StableID == "" || rule.Invariant == "" || rule.State == "" || rule.Reason == "" || rule.UnknownClass == "" || rule.NextOperation == "" {
		return RuleDecl{}, fmt.Errorf("line %d: incomplete rule", line)
	}
	if rule.State != StateClosed && rule.State != StateUnknown && rule.State != StateRefuted {
		return RuleDecl{}, fmt.Errorf("line %d: invalid rule state %s", line, rule.State)
	}
	return rule, nil
}

func parseCase(values map[string]string, location SourceLocation, line int) (CaseContract, error) {
	ordinal, err := integerValue(values, "ordinal")
	if err != nil || ordinal < 1 {
		return CaseContract{}, fmt.Errorf("line %d: invalid case ordinal", line)
	}
	caseContract := CaseContract{Ordinal: ordinal, StableID: values["id"], Expected: values["expected"], Fixture: values["fixture"], Source: location}
	if caseContract.StableID == "" || caseContract.Expected == "" || caseContract.Fixture == "" {
		return CaseContract{}, fmt.Errorf("line %d: incomplete case", line)
	}
	if caseContract.Expected != StateClosed && caseContract.Expected != StateUnknown && caseContract.Expected != StateRefuted {
		return CaseContract{}, fmt.Errorf("line %d: invalid case state %s", line, caseContract.Expected)
	}
	return caseContract, nil
}

func validateGraph(graph SemanticGraph) error {
	if graph.Schema != GraphSchema || graph.GraphID == "" || graph.Release == "" || graph.LedgerDigest == "" {
		return errors.New("graph declaration is incomplete")
	}
	if !equalStrings(graph.Precedence, []string{StateRefuted, StateUnknown, StateClosed}) {
		return errors.New("graph precedence must be REFUTED,UNKNOWN,CLOSED")
	}
	if graph.RepositoryWrites != 0 || graph.LocalTestExecutions != 0 || graph.CrossProjectRequiredGates != 0 {
		return errors.New("graph runtime contract must keep repository writes, local tests, and cross-project gates at zero")
	}
	if graph.OutputArtifactCount != len(RequiredArtifactNames) || len(graph.Artifacts) != len(RequiredArtifactNames) {
		return errors.New("graph must declare exactly six output artifacts")
	}
	if err := validateOrdinals("artifact", artifactOrdinals(graph.Artifacts), len(RequiredArtifactNames)); err != nil {
		return err
	}
	artifactNames := make([]string, 0, len(graph.Artifacts))
	for _, artifact := range graph.Artifacts {
		artifactNames = append(artifactNames, artifact.Name)
	}
	if !equalStrings(artifactNames, RequiredArtifactNames) {
		return errors.New("graph artifact names must match the fixed six-artifact contract")
	}
	if len(graph.Fields) != len(RequiredProposalFields) {
		return errors.New("graph must declare exactly eight proposal fields")
	}
	if err := validateOrdinals("field", fieldOrdinals(graph.Fields), len(RequiredProposalFields)); err != nil {
		return err
	}
	fieldNames := make([]string, 0, len(graph.Fields))
	for _, field := range graph.Fields {
		if !field.Required {
			return fmt.Errorf("proposal field %s must be required", field.Name)
		}
		fieldNames = append(fieldNames, field.Name)
	}
	if !equalStrings(fieldNames, RequiredProposalFields) {
		return errors.New("graph proposal fields do not match the required envelope fields")
	}
	if len(graph.States) != 3 || graph.States[StateClosed].Result != ResultMergeable || graph.States[StateUnknown].Result != StateUnknown || graph.States[StateRefuted].Result != StateRefuted {
		return errors.New("graph must declare CLOSED, UNKNOWN, and REFUTED state projections")
	}
	if len(graph.Authorities) == 0 {
		return errors.New("graph must declare an authority scope")
	}
	if graph.BootstrapProvenance.State != StateRefuted || graph.BootstrapProvenance.Reason != "PR_FIRST_IMPLEMENTATION_BYPASSED" || graph.BootstrapProvenance.Commit == "" || graph.BootstrapProvenance.Ref == "" {
		return errors.New("graph must preserve the direct-main bootstrap refutation provenance")
	}
	if !graph.ReviewGate.Required || graph.ReviewGate.MissingState != StateUnknown || graph.ReviewGate.ReviewedState == "" || graph.ReviewGate.MergedState == "" || graph.ReviewGate.Reason == "" || graph.ReviewGate.UnknownClass == "" || graph.ReviewGate.NextOperation == "" || graph.ReviewFixture == "" {
		return errors.New("graph must declare a required PR-reviewed release gate")
	}
	if len(graph.HistoricalReleases) != 1 || graph.HistoricalReleases[0].Tag != "v0.1.0" || graph.HistoricalReleases[0].State != StateRefuted || graph.HistoricalReleases[0].Run == "" || graph.HistoricalReleases[0].Reason == "" {
		return errors.New("graph must preserve the failed v0.1.0 release attempt")
	}
	if len(graph.Invariants) != 12 {
		return errors.New("graph denominator must contain exactly twelve invariants")
	}
	if err := validateOrdinals("invariant", invariantOrdinals(graph.Invariants), 12); err != nil {
		return err
	}
	proofCounts := map[string]int{}
	indicatorCounts := map[string]int{}
	for _, invariant := range graph.Invariants {
		proofCounts[invariant.Proof]++
		indicatorCounts[invariant.Indicator]++
	}
	for _, label := range []string{"FOUNDATION", "COHERENCE", "REGRESSION"} {
		if proofCounts[label] != 4 {
			return fmt.Errorf("proof vector must contain four %s invariants", label)
		}
	}
	for _, label := range []string{"DRIVER", "OUTCOME", "GUARDRAIL"} {
		if indicatorCounts[label] != 4 {
			return fmt.Errorf("indicator vector must contain four %s invariants", label)
		}
	}
	if len(graph.Rules) == 0 {
		return errors.New("graph must declare projection rules")
	}
	invariantIDs := map[string]bool{}
	for _, invariant := range graph.Invariants {
		invariantIDs[invariant.StableID] = true
	}
	for id, rule := range graph.Rules {
		if rule.StableID != id || !invariantIDs[rule.Invariant] {
			return fmt.Errorf("rule %s references an unknown invariant", id)
		}
	}
	if len(graph.Cases) != 12 {
		return errors.New("graph must declare exactly twelve canonical cases")
	}
	if err := validateOrdinals("case", caseOrdinals(graph.Cases), 12); err != nil {
		return err
	}
	stateCounts := map[string]int{}
	fixtures := map[string]bool{}
	for _, caseContract := range graph.Cases {
		stateCounts[caseContract.Expected]++
		if fixtures[caseContract.Fixture] {
			return fmt.Errorf("duplicate fixture %s", caseContract.Fixture)
		}
		fixtures[caseContract.Fixture] = true
	}
	if stateCounts[StateClosed] != 4 || stateCounts[StateUnknown] != 4 || stateCounts[StateRefuted] != 4 {
		return errors.New("canonical case distribution must be four CLOSED, four UNKNOWN, four REFUTED")
	}
	return nil
}

func LoadCases(directory string, graph SemanticGraph) ([]FixtureInput, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	byFixture := map[string]CaseContract{}
	for _, caseContract := range graph.Cases {
		byFixture[caseContract.Fixture] = caseContract
	}
	fixtures := make([]FixtureInput, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		fixture, err := decodeFixture(path)
		if err != nil {
			return nil, err
		}
		contract, ok := byFixture[entry.Name()]
		if !ok {
			return nil, fmt.Errorf("fixture %s is not declared by the graph", entry.Name())
		}
		if fixture.FixtureID != contract.StableID || fixture.ExpectedState != contract.Expected {
			return nil, fmt.Errorf("fixture %s identity or expected state disagrees with .gooo", entry.Name())
		}
		if seen[fixture.FixtureID] {
			return nil, fmt.Errorf("duplicate fixture id %s", fixture.FixtureID)
		}
		seen[fixture.FixtureID] = true
		fixture.FixturePath = filepath.ToSlash(path)
		fixtures = append(fixtures, fixture)
	}
	if len(fixtures) != len(graph.Cases) {
		return nil, fmt.Errorf("loaded %d fixtures, expected %d", len(fixtures), len(graph.Cases))
	}
	sort.Slice(fixtures, func(i, j int) bool {
		return caseOrdinal(graph, fixtures[i].FixtureID) < caseOrdinal(graph, fixtures[j].FixtureID)
	})
	return fixtures, nil
}

func LoadReviewFixture(path string, graph SemanticGraph) (ReviewFixture, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ReviewFixture{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var fixture ReviewFixture
	if err := decoder.Decode(&fixture); err != nil {
		return ReviewFixture{}, fmt.Errorf("decode review fixture %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ReviewFixture{}, fmt.Errorf("decode review fixture %s: multiple JSON values", path)
		}
		return ReviewFixture{}, fmt.Errorf("decode review fixture %s: %w", path, err)
	}
	if fixture.FixtureID != "pr-reviewed-release" || fixture.Kind != "operator-provenance-gate" || !fixture.Required || fixture.BootstrapState != graph.BootstrapProvenance.State || fixture.BootstrapReason != graph.BootstrapProvenance.Reason || fixture.MissingReviewState != graph.ReviewGate.MissingState || fixture.ReviewedState != graph.ReviewGate.ReviewedState || fixture.MergedState != graph.ReviewGate.MergedState || fixture.EvidenceSource == "" || !fixture.FailClosed {
		return ReviewFixture{}, errors.New("review fixture does not close the released PR-reviewed gate")
	}
	return fixture, nil
}

func decodeFixture(path string) (FixtureInput, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return FixtureInput{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var fixture FixtureInput
	if err := decoder.Decode(&fixture); err != nil {
		return FixtureInput{}, fmt.Errorf("decode %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return FixtureInput{}, fmt.Errorf("decode %s: multiple JSON values", path)
		}
		return FixtureInput{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return fixture, nil
}

func keyValues(tokens []string) (map[string]string, error) {
	values := map[string]string{}
	for _, token := range tokens {
		parts := strings.SplitN(token, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("invalid key=value token %q", token)
		}
		if _, exists := values[parts[0]]; exists {
			return nil, fmt.Errorf("duplicate key %s", parts[0])
		}
		values[parts[0]] = parts[1]
	}
	return values, nil
}

func splitList(value string) []string {
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func integerValue(values map[string]string, key string) (int, error) {
	value, ok := values[key]
	if !ok {
		return 0, fmt.Errorf("missing integer %s", key)
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %s", key)
	}
	return parsed, nil
}

func boolValue(values map[string]string, key string) (bool, error) {
	value, ok := values[key]
	if !ok {
		return false, fmt.Errorf("missing boolean %s", key)
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid boolean %s", key)
	}
	return parsed, nil
}

func stripComment(line string) string {
	if index := strings.IndexByte(line, '#'); index >= 0 {
		return line[:index]
	}
	return line
}

func registerID(seen map[string]string, id, kind string, line int) error {
	if id == "" {
		return fmt.Errorf("line %d: empty %s id", line, kind)
	}
	if previous, exists := seen[id]; exists {
		return fmt.Errorf("line %d: duplicate semantic id %s (%s and %s)", line, id, previous, kind)
	}
	seen[id] = kind
	return nil
}

func validateOrdinals(kind string, ordinals []int, expected int) error {
	if len(ordinals) != expected {
		return fmt.Errorf("%s count is %d, expected %d", kind, len(ordinals), expected)
	}
	sort.Ints(ordinals)
	for index, ordinal := range ordinals {
		if ordinal != index+1 {
			return fmt.Errorf("%s ordinals must be contiguous from one", kind)
		}
	}
	return nil
}

func artifactOrdinals(values []ArtifactDecl) []int {
	result := make([]int, 0, len(values))
	for _, value := range values {
		result = append(result, value.Ordinal)
	}
	return result
}

func fieldOrdinals(values []FieldDecl) []int {
	result := make([]int, 0, len(values))
	for _, value := range values {
		result = append(result, value.Ordinal)
	}
	return result
}

func invariantOrdinals(values []InvariantDecl) []int {
	result := make([]int, 0, len(values))
	for _, value := range values {
		result = append(result, value.Ordinal)
	}
	return result
}

func caseOrdinals(values []CaseContract) []int {
	result := make([]int, 0, len(values))
	for _, value := range values {
		result = append(result, value.Ordinal)
	}
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func caseOrdinal(graph SemanticGraph, caseID string) int {
	for _, caseContract := range graph.Cases {
		if caseContract.StableID == caseID {
			return caseContract.Ordinal
		}
	}
	return len(graph.Cases) + 1
}

func invariantByID(graph SemanticGraph, id string) InvariantDecl {
	for _, invariant := range graph.Invariants {
		if invariant.StableID == id {
			return invariant
		}
	}
	return InvariantDecl{}
}
