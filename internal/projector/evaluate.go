package projector

import (
	"fmt"
	"sort"
	"strings"
)

type proposalRecord struct {
	input  ProposalInput
	issues []Issue
}

func EvaluateFixtures(ir SemanticIR, fixtures []FixtureInput) []CaseResult {
	results := make([]CaseResult, 0, len(fixtures))
	for _, fixture := range fixtures {
		results = append(results, EvaluateFixture(ir, fixture))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Ordinal < results[j].Ordinal })
	return results
}

func EvaluateFixture(ir SemanticIR, fixture FixtureInput) CaseResult {
	caseContract := caseByID(ir.Graph, fixture.FixtureID)
	result := CaseResult{
		Ordinal:                       caseContract.Ordinal,
		InvariantID:                   invariantIDForOrdinal(ir.Graph, caseContract.Ordinal),
		CaseID:                        fixture.FixtureID,
		ExpectedState:                 caseContract.Expected,
		State:                         StateClosed,
		Result:                        ir.Graph.States[StateClosed].Result,
		BaseLedgerDigest:              fixture.BaseLedgerDigest,
		DeterministicTopologicalOrder: []string{},
		TopologicalOrderValid:         true,
		AcceptedWave:                  []string{},
		DeferredFrontier:              []DeferredProposal{},
		ConflictWitnesses:             []ConflictWitness{},
		Proposals:                     []ProposalProjection{},
		NextOperation:                 ir.Graph.States[StateClosed].NextOperation,
		FixturePath:                   fixture.FixturePath,
	}

	targetBase := ir.Graph.LedgerDigest
	if fixture.BaseLedgerDigest != targetBase {
		result.ConflictWitnesses = append(result.ConflictWitnesses, ConflictWitness{
			RuleID: "base_digest_mismatch", State: StateUnknown, LeftProposal: "fixture",
			RightProposal: "released-ledger", Resource: "base_ledger_digest",
			Evidence: []string{"expected_base=" + targetBase, "observed_base=" + fixture.BaseLedgerDigest},
		})
	}

	records := make([]proposalRecord, 0, len(fixture.Proposals))
	byID := map[string][]int{}
	evidence := makeStringSet(fixture.EvidenceDigests)
	for index, proposal := range fixture.Proposals {
		record := proposalRecord{input: proposal, issues: []Issue{}}
		validateEnvelope(ir.Graph, &record, targetBase, evidence)
		records = append(records, record)
		if proposal.ProposalID != "" {
			byID[proposal.ProposalID] = append(byID[proposal.ProposalID], index)
		}
	}
	if len(records) == 0 {
		result.State = StateRefuted
		result.Result = ir.Graph.States[StateRefuted].Result
		result.Reason = ir.Graph.Rules["empty_proposal_set"].Reason
		result.NextOperation = ir.Graph.Rules["empty_proposal_set"].NextOperation
		result.ConflictWitnesses = append(result.ConflictWitnesses, ConflictWitness{
			RuleID: "empty_proposal_set", State: StateRefuted, LeftProposal: "proposal-set",
			RightProposal: "", Resource: "proposals", Evidence: []string{"observed_count=0"},
		})
		return result
	}

	for id, indexes := range byID {
		if id == "" || len(indexes) < 2 {
			continue
		}
		sortedIndexes := append([]int(nil), indexes...)
		sort.Ints(sortedIndexes)
		for _, index := range sortedIndexes {
			records[index].issues = append(records[index].issues, issueForRule(ir.Graph, "duplicate_proposal_id", []string{id}, []string{"duplicate_proposal_id=" + id}))
		}
		result.ConflictWitnesses = append(result.ConflictWitnesses, ConflictWitness{
			RuleID: "duplicate_proposal_id", State: StateRefuted, LeftProposal: id,
			RightProposal: id, Resource: "proposal_id", Evidence: []string{"duplicate_proposal_id=" + id},
		})
	}

	dependencies := map[string][]string{}
	uniqueIDs := make([]string, 0, len(byID))
	for id, indexes := range byID {
		if id == "" || len(indexes) != 1 {
			continue
		}
		uniqueIDs = append(uniqueIDs, id)
		dependencies[id] = sortedCopy(records[indexes[0]].input.CausalDependencies)
	}
	sort.Strings(uniqueIDs)

	for _, id := range uniqueIDs {
		index := byID[id][0]
		for _, dependency := range dependencies[id] {
			if len(byID[dependency]) != 1 {
				records[index].issues = append(records[index].issues, issueForRule(ir.Graph, "missing_dependency", []string{dependency}, []string{"missing_dependency=" + dependency}))
			}
		}
	}

	cyclePaths := dependencyCycles(uniqueIDs, dependencies)
	for _, cycle := range cyclePaths {
		cycleEvidence := []string{"cycle_path=" + strings.Join(cycle, ">")}
		for _, id := range cycleMembers(cycle) {
			if indexes := byID[id]; len(indexes) == 1 {
				records[indexes[0]].issues = append(records[indexes[0]].issues, issueForRule(ir.Graph, "dependency_cycle", cycleMembers(cycle), cycleEvidence))
			}
		}
		left, right := cycle[0], cycle[0]
		if len(cycle) > 1 {
			right = cycle[1]
		}
		result.ConflictWitnesses = append(result.ConflictWitnesses, ConflictWitness{
			RuleID: "dependency_cycle", State: StateUnknown, LeftProposal: left,
			RightProposal: right, Resource: "causal_dependencies", Evidence: cycleEvidence,
		})
	}

	for leftIndex := 0; leftIndex < len(uniqueIDs); leftIndex++ {
		leftID := uniqueIDs[leftIndex]
		left := records[byID[leftID][0]].input
		for rightIndex := leftIndex + 1; rightIndex < len(uniqueIDs); rightIndex++ {
			rightID := uniqueIDs[rightIndex]
			right := records[byID[rightID][0]].input
			if resource := overlappingResource(left.SemanticWriteSet, right.SemanticWriteSet); resource != "" {
				witness := ConflictWitness{
					RuleID: "write_conflict", State: StateRefuted, LeftProposal: leftID,
					RightProposal: rightID, Resource: resource,
					Evidence: []string{"left_write=" + strings.Join(sortStringsUnique(left.SemanticWriteSet), ","), "right_write=" + strings.Join(sortStringsUnique(right.SemanticWriteSet), ",")},
				}
				result.ConflictWitnesses = append(result.ConflictWitnesses, witness)
				records[byID[leftID][0]].issues = append(records[byID[leftID][0]].issues, issueForRule(ir.Graph, "write_conflict", []string{leftID, rightID, resource}, witness.Evidence))
				records[byID[rightID][0]].issues = append(records[byID[rightID][0]].issues, issueForRule(ir.Graph, "write_conflict", []string{leftID, rightID, resource}, witness.Evidence))
			}
			if resource := overlappingResource(left.SemanticWriteSet, right.SemanticReadSet); resource != "" && !causallyBefore(dependencies, leftID, rightID) {
				witness := ConflictWitness{
					RuleID: "read_write_conflict", State: StateRefuted, LeftProposal: leftID,
					RightProposal: rightID, Resource: resource,
					Evidence: []string{"writer=" + leftID, "reader=" + rightID, "resource=" + resource, "causal_order=absent"},
				}
				result.ConflictWitnesses = append(result.ConflictWitnesses, witness)
				records[byID[leftID][0]].issues = append(records[byID[leftID][0]].issues, issueForRule(ir.Graph, "read_write_conflict", []string{leftID, rightID, resource}, witness.Evidence))
				records[byID[rightID][0]].issues = append(records[byID[rightID][0]].issues, issueForRule(ir.Graph, "read_write_conflict", []string{leftID, rightID, resource}, witness.Evidence))
			}
			if resource := overlappingResource(right.SemanticWriteSet, left.SemanticReadSet); resource != "" && !causallyBefore(dependencies, rightID, leftID) {
				witness := ConflictWitness{
					RuleID: "read_write_conflict", State: StateRefuted, LeftProposal: rightID,
					RightProposal: leftID, Resource: resource,
					Evidence: []string{"writer=" + rightID, "reader=" + leftID, "resource=" + resource, "causal_order=absent"},
				}
				result.ConflictWitnesses = append(result.ConflictWitnesses, witness)
				records[byID[leftID][0]].issues = append(records[byID[leftID][0]].issues, issueForRule(ir.Graph, "read_write_conflict", []string{rightID, leftID, resource}, witness.Evidence))
				records[byID[rightID][0]].issues = append(records[byID[rightID][0]].issues, issueForRule(ir.Graph, "read_write_conflict", []string{rightID, leftID, resource}, witness.Evidence))
			}
		}
	}

	for _, id := range uniqueIDs {
		index := byID[id][0]
		if proposalState(records[index].issues) != StateClosed {
			continue
		}
		for _, dependency := range dependencies[id] {
			dependencyIndexes := byID[dependency]
			if len(dependencyIndexes) != 1 || proposalState(records[dependencyIndexes[0]].issues) != StateClosed {
				records[index].issues = append(records[index].issues, issueForRule(ir.Graph, "dependency_not_closed", []string{dependency}, []string{"blocked_by_proposal=" + dependency}))
			}
		}
	}

	order, orderValid := topologicalOrder(uniqueIDs, dependencies)
	result.DeterministicTopologicalOrder = order
	result.TopologicalOrderValid = orderValid
	accepted := map[string]bool{}
	for _, id := range uniqueIDs {
		index := byID[id][0]
		if proposalState(records[index].issues) != StateClosed {
			continue
		}
		closedDependencies := true
		for _, dependency := range dependencies[id] {
			dependencyIndexes := byID[dependency]
			if len(dependencyIndexes) != 1 || proposalState(records[dependencyIndexes[0]].issues) != StateClosed {
				closedDependencies = false
			}
		}
		if closedDependencies {
			accepted[id] = true
		}
	}
	for _, id := range order {
		if accepted[id] {
			result.AcceptedWave = append(result.AcceptedWave, id)
		}
	}

	proposalIndexes := make([]int, len(records))
	for index := range records {
		proposalIndexes[index] = index
	}
	sort.SliceStable(proposalIndexes, func(i, j int) bool {
		left, right := records[proposalIndexes[i]].input.ProposalID, records[proposalIndexes[j]].input.ProposalID
		if left == right {
			return proposalIndexes[i] < proposalIndexes[j]
		}
		return left < right
	})
	for _, index := range proposalIndexes {
		record := records[index]
		state := proposalState(record.issues)
		var unknown *UnknownClaim
		if state == StateUnknown {
			unknownIssue := firstIssueWithState(record.issues, StateUnknown)
			unknown = unknownForIssue(ir.Graph, unknownIssue)
		}
		result.Proposals = append(result.Proposals, ProposalProjection{
			ProposalID:     record.input.ProposalID,
			State:          state,
			Result:         stateResult(ir.Graph, state),
			Unknown:        unknown,
			Issues:         sortedIssues(record.issues),
			Dependencies:   sortStringsUnique(record.input.CausalDependencies),
			ReadSet:        sortStringsUnique(record.input.SemanticReadSet),
			WriteSet:       sortStringsUnique(record.input.SemanticWriteSet),
			AuthorityScope: sortStringsUnique(record.input.AuthorityScope),
		})
		if !accepted[record.input.ProposalID] {
			issue := primaryIssue(record.issues)
			result.DeferredFrontier = append(result.DeferredFrontier, DeferredProposal{
				ProposalID: record.input.ProposalID,
				State:      state,
				Reason:     issue.Reason,
				BlockedBy:  sortedNonNil(issue.BlockedBy),
			})
		}
	}

	result.ConflictWitnesses = sortedWitnesses(result.ConflictWitnesses)
	allIssues := make([]Issue, 0)
	if fixture.BaseLedgerDigest != targetBase {
		allIssues = append(allIssues, issueForRule(ir.Graph, "base_digest_mismatch", []string{"base_ledger_digest"}, []string{"expected_base=" + targetBase, "observed_base=" + fixture.BaseLedgerDigest}))
	}
	for _, record := range records {
		allIssues = append(allIssues, record.issues...)
	}
	result.State = proposalState(allIssues)
	result.Result = stateResult(ir.Graph, result.State)
	if result.State == StateClosed {
		result.Reason = "ALL_PROPOSALS_SERIALIZABLE_AND_AUTHORITY_CLOSED"
		result.NextOperation = ir.Graph.States[StateClosed].NextOperation
	} else if result.State == StateUnknown {
		issue := firstIssueWithState(allIssues, StateUnknown)
		result.Reason = issue.Reason
		result.Unknown = unknownForIssue(ir.Graph, issue)
		result.NextOperation = unknownForIssue(ir.Graph, issue).NextOperation
	} else {
		issue := firstIssueWithState(allIssues, StateRefuted)
		result.Reason = issue.Reason
		result.NextOperation = ruleNextOperation(ir.Graph, issue.RuleID)
	}
	return result
}

func validateEnvelope(graph SemanticGraph, record *proposalRecord, targetBase string, evidence map[string]bool) {
	proposal := record.input
	missing := []string{}
	if proposal.ProposalID == "" {
		missing = append(missing, "proposal_id")
	}
	if proposal.BaseLedgerDigest == "" {
		missing = append(missing, "base_ledger_digest")
	}
	if proposal.SemanticReadSet == nil {
		missing = append(missing, "semantic_read_set")
	}
	if proposal.SemanticWriteSet == nil {
		missing = append(missing, "semantic_write_set")
	}
	if proposal.RequiredEvidence == nil {
		missing = append(missing, "required_evidence_digests")
	}
	if proposal.ToolReleaseLocks == nil {
		missing = append(missing, "tool_release_locks")
	}
	if proposal.CausalDependencies == nil {
		missing = append(missing, "causal_dependencies")
	}
	if proposal.AuthorityScope == nil {
		missing = append(missing, "authority_scope")
	}
	if len(missing) > 0 || len(proposal.ToolReleaseLocks) == 0 || len(proposal.AuthorityScope) == 0 {
		if len(missing) == 0 {
			missing = append(missing, "non_empty_tool_release_locks_or_authority_scope")
		}
		record.issues = append(record.issues, issueForRule(graph, "malformed_proposal", []string{"proposal_envelope"}, []string{"missing_or_empty_field=" + strings.Join(missing, ",")}))
		return
	}
	if hasDuplicates(proposal.SemanticReadSet) || hasDuplicates(proposal.SemanticWriteSet) || hasDuplicates(proposal.RequiredEvidence) || hasDuplicates(proposal.CausalDependencies) || hasDuplicates(proposal.AuthorityScope) {
		record.issues = append(record.issues, issueForRule(graph, "malformed_proposal", []string{"duplicate_envelope_value"}, []string{"proposal_id=" + proposal.ProposalID}))
	}
	if proposal.BaseLedgerDigest != targetBase {
		record.issues = append(record.issues, issueForRule(graph, "base_digest_mismatch", []string{"base_ledger_digest"}, []string{"expected_base=" + targetBase, "observed_base=" + proposal.BaseLedgerDigest}))
	}
	for _, required := range sortStringsUnique(proposal.RequiredEvidence) {
		if !evidence[required] {
			record.issues = append(record.issues, issueForRule(graph, "missing_evidence", []string{required}, []string{"missing_evidence_digest=" + required}))
		}
	}
	for _, lock := range proposal.ToolReleaseLocks {
		if lock.ToolID == "" || lock.ReleaseDigest == "" {
			record.issues = append(record.issues, issueForRule(graph, "malformed_proposal", []string{"tool_release_locks"}, []string{"tool_id=" + lock.ToolID, "release_digest=" + lock.ReleaseDigest}))
			continue
		}
		if lock.Mutable != nil && *lock.Mutable {
			record.issues = append(record.issues, issueForRule(graph, "mutable_tool_lock", []string{lock.ToolID}, []string{"tool_id=" + lock.ToolID, "mutable=true", "release_digest=" + lock.ReleaseDigest}))
		}
		if lock.Verified == nil || !*lock.Verified {
			record.issues = append(record.issues, issueForRule(graph, "unverified_tool_lock", []string{lock.ToolID}, []string{"tool_id=" + lock.ToolID, "verified=false", "release_digest=" + lock.ReleaseDigest}))
		}
	}
	allowed := allowedAuthority(graph)
	for _, scope := range sortStringsUnique(proposal.AuthorityScope) {
		if !allowed[scope] {
			record.issues = append(record.issues, issueForRule(graph, "authority_escalation", []string{scope}, []string{"requested_authority=" + scope, "allowed_authority=" + strings.Join(sortedAuthority(graph), ",")}))
		}
	}
}

func issueForRule(graph SemanticGraph, ruleID string, blockedBy, evidence []string) Issue {
	rule := graph.Rules[ruleID]
	return Issue{RuleID: ruleID, State: rule.State, Reason: rule.Reason, Evidence: sortedNonNil(evidence), BlockedBy: sortedNonNil(blockedBy)}
}

func unknownForIssue(graph SemanticGraph, issue Issue) *UnknownClaim {
	rule := graph.Rules[issue.RuleID]
	invariant := invariantByID(graph, rule.Invariant)
	return &UnknownClaim{
		Stage:         invariant.Stage,
		Step:          invariant.Step,
		Reason:        issue.Reason,
		UnknownClass:  rule.UnknownClass,
		NextOperation: rule.NextOperation,
		BlockedBy:     sortedNonNil(issue.BlockedBy),
	}
}

func proposalState(issues []Issue) string {
	state := StateClosed
	for _, issue := range issues {
		if stateRank(issue.State) > stateRank(state) {
			state = issue.State
		}
	}
	return state
}

func stateRank(state string) int {
	switch state {
	case StateRefuted:
		return 3
	case StateUnknown:
		return 2
	case StateClosed:
		return 1
	default:
		return 4
	}
}

func stateResult(graph SemanticGraph, state string) string {
	if declaration, ok := graph.States[state]; ok {
		return declaration.Result
	}
	return StateRefuted
}

func ruleNextOperation(graph SemanticGraph, ruleID string) string {
	if rule, ok := graph.Rules[ruleID]; ok {
		return rule.NextOperation
	}
	return graph.States[StateRefuted].NextOperation
}

func primaryIssue(issues []Issue) Issue {
	if len(issues) == 0 {
		return Issue{State: StateClosed, Reason: "ALL_PROPOSALS_SERIALIZABLE_AND_AUTHORITY_CLOSED", BlockedBy: []string{}, Evidence: []string{}}
	}
	sorted := sortedIssues(issues)
	return sorted[0]
}

func firstIssueWithState(issues []Issue, state string) Issue {
	filtered := make([]Issue, 0)
	for _, issue := range issues {
		if issue.State == state {
			filtered = append(filtered, issue)
		}
	}
	if len(filtered) == 0 {
		return Issue{State: state, Reason: fmt.Sprintf("NO_%s_REASON", state), BlockedBy: []string{}, Evidence: []string{}}
	}
	return primaryIssue(filtered)
}

func sortedIssues(issues []Issue) []Issue {
	result := append([]Issue(nil), issues...)
	sort.SliceStable(result, func(i, j int) bool {
		if stateRank(result[i].State) != stateRank(result[j].State) {
			return stateRank(result[i].State) > stateRank(result[j].State)
		}
		if result[i].RuleID != result[j].RuleID {
			return result[i].RuleID < result[j].RuleID
		}
		return strings.Join(result[i].Evidence, "|") < strings.Join(result[j].Evidence, "|")
	})
	return result
}

func sortedWitnesses(witnesses []ConflictWitness) []ConflictWitness {
	result := append([]ConflictWitness(nil), witnesses...)
	sort.SliceStable(result, func(i, j int) bool {
		left := result[i].RuleID + "|" + result[i].LeftProposal + "|" + result[i].RightProposal + "|" + result[i].Resource
		right := result[j].RuleID + "|" + result[j].LeftProposal + "|" + result[j].RightProposal + "|" + result[j].Resource
		return left < right
	})
	return result
}

func sortedNonNil(values []string) []string {
	result := sortedCopy(values)
	if result == nil {
		return []string{}
	}
	return result
}

func makeStringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}

func hasDuplicates(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}

func overlappingResource(left, right []string) string {
	rightSet := makeStringSet(right)
	resources := []string{}
	for _, value := range left {
		if rightSet[value] {
			resources = append(resources, value)
		}
	}
	resources = sortStringsUnique(resources)
	if len(resources) == 0 {
		return ""
	}
	return resources[0]
}

func allowedAuthority(graph SemanticGraph) map[string]bool {
	result := map[string]bool{}
	for _, authority := range graph.Authorities {
		for _, scope := range authority.Scope {
			result[scope] = true
		}
	}
	return result
}

func sortedAuthority(graph SemanticGraph) []string {
	values := []string{}
	for _, authority := range graph.Authorities {
		values = append(values, authority.Scope...)
	}
	return sortStringsUnique(values)
}

func causallyBefore(dependencies map[string][]string, before, after string) bool {
	visited := map[string]bool{}
	var visit func(string) bool
	visit = func(current string) bool {
		if visited[current] {
			return false
		}
		visited[current] = true
		for _, dependency := range dependencies[current] {
			if dependency == before || visit(dependency) {
				return true
			}
		}
		return false
	}
	return visit(after)
}

func dependencyCycles(ids []string, dependencies map[string][]string) [][]string {
	color := map[string]int{}
	stack := []string{}
	position := map[string]int{}
	cycles := map[string][]string{}
	var visit func(string)
	visit = func(current string) {
		color[current] = 1
		position[current] = len(stack)
		stack = append(stack, current)
		deps := sortedCopy(dependencies[current])
		for _, dependency := range deps {
			if color[dependency] == 0 {
				visit(dependency)
				continue
			}
			if color[dependency] == 1 {
				path := append([]string(nil), stack[position[dependency]:]...)
				path = append(path, dependency)
				members := cycleMembers(path)
				key := strings.Join(members, "|")
				cycles[key] = append([]string(nil), path...)
			}
		}
		stack = stack[:len(stack)-1]
		delete(position, current)
		color[current] = 2
	}
	for _, id := range ids {
		if color[id] == 0 {
			visit(id)
		}
	}
	keys := make([]string, 0, len(cycles))
	for key := range cycles {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([][]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, cycles[key])
	}
	return result
}

func cycleMembers(path []string) []string {
	return sortStringsUnique(path)
}

func topologicalOrder(ids []string, dependencies map[string][]string) ([]string, bool) {
	indegree := map[string]int{}
	edges := map[string][]string{}
	for _, id := range ids {
		indegree[id] = 0
		edges[id] = []string{}
	}
	for _, id := range ids {
		for _, dependency := range dependencies[id] {
			if _, ok := indegree[dependency]; !ok {
				continue
			}
			indegree[id]++
			edges[dependency] = append(edges[dependency], id)
		}
	}
	ready := []string{}
	for _, id := range ids {
		if indegree[id] == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	order := []string{}
	for len(ready) > 0 {
		current := ready[0]
		ready = ready[1:]
		order = append(order, current)
		children := sortStringsUnique(edges[current])
		for _, child := range children {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
			}
		}
		sort.Strings(ready)
	}
	valid := len(order) == len(ids)
	if !valid {
		remaining := []string{}
		seen := makeStringSet(order)
		for _, id := range ids {
			if !seen[id] {
				remaining = append(remaining, id)
			}
		}
		order = append(order, sortedCopy(remaining)...)
	}
	return order, valid
}

func caseByID(graph SemanticGraph, id string) CaseContract {
	for _, caseContract := range graph.Cases {
		if caseContract.StableID == id {
			return caseContract
		}
	}
	return CaseContract{}
}

func invariantIDForOrdinal(graph SemanticGraph, ordinal int) string {
	for _, invariant := range graph.Invariants {
		if invariant.Ordinal == ordinal {
			return invariant.StableID
		}
	}
	return ""
}
