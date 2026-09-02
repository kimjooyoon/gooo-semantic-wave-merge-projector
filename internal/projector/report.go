package projector

import (
	"fmt"
	"strings"
)

func renderReport(ir SemanticIR, results []CaseResult, replay ReplayReceipt, provenance OperatorProvenanceReceipt) string {
	var builder strings.Builder
	counts := stateCounts(results)
	proof := vectorCounts(ir.Graph, true)
	indicators := vectorCounts(ir.Graph, false)
	builder.WriteString("# Gooo semantic wave merge projector report\n\n")
	fmt.Fprintf(&builder, "Graph: `%s`\n\nReleased graph digest: `%s`\n\n", ir.Graph.GraphID, ir.SourceDigest)
	builder.WriteString("This is a Gooo self-improvement proposal serializer. It does not perform a Git merge or commit. Go supplies parser, evaluator, generator, and runtime behavior; the released `.gooo` graph supplies semantic authority.\n\n")
	fmt.Fprintf(&builder, "Runtime contract: repository writes `%d`; local test executions `%d`; cross-project required gates `%d`; output artifacts `%d`.\n\n", ir.Graph.RepositoryWrites, ir.Graph.LocalTestExecutions, ir.Graph.CrossProjectRequiredGates, len(RequiredArtifactNames))
	fmt.Fprintf(&builder, "Operator provenance: bootstrap `%s` (`%s`) at commit `%s` on ref `%s`. This fact remains REFUTED because PR-first implementation was bypassed. Historical release `%s` is `%s` (`%s`) from Actions run `%s`. Review gate: `%s`; pull request `%d`.\n\n", provenance.BootstrapState, provenance.BootstrapReason, provenance.BootstrapCommit, provenance.BootstrapRef, provenance.HistoricalRelease, provenance.HistoricalState, provenance.HistoricalReason, provenance.HistoricalRun, provenance.ReviewGate, provenance.PullRequestNumber)
	builder.WriteString("## Fixed denominator\n\n")
	fmt.Fprintf(&builder, "There are exactly `%d` named immutable conditions. State counts are `CLOSED=%d`, `UNKNOWN=%d`, `REFUTED=%d`; precedence is `%s`.\n\n", len(ir.Graph.Invariants), counts.Closed, counts.Unknown, counts.Refuted, strings.Join(ir.Graph.Precedence, " > "))
	builder.WriteString("| proof vector | count |\n|---|---:|\n")
	for _, value := range proof {
		fmt.Fprintf(&builder, "| %s | %d |\n", value.Label, value.Count)
	}
	builder.WriteString("\n| indicator vector | count |\n|---|---:|\n")
	for _, value := range indicators {
		fmt.Fprintf(&builder, "| %s | %d |\n", value.Label, value.Count)
	}
	builder.WriteString("\nThese are direct integer counts from the released graph. No aggregate metric is emitted.\n\n")

	builder.WriteString("## Wave projections\n\n")
	builder.WriteString("| ordinal | case | expected | state | result | accepted wave | deferred frontier | next operation |\n|---:|---|---|---|---|---|---|---|\n")
	for _, result := range results {
		deferred := make([]string, 0, len(result.DeferredFrontier))
		for _, proposal := range result.DeferredFrontier {
			deferred = append(deferred, proposal.ProposalID+":"+proposal.State)
		}
		fmt.Fprintf(&builder, "| %d | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` |\n", result.Ordinal, result.CaseID, result.ExpectedState, result.State, result.Result, strings.Join(result.AcceptedWave, ","), strings.Join(deferred, ","), result.NextOperation)
	}
	builder.WriteString("\n")

	builder.WriteString("## Evidence and witnesses\n\n")
	builder.WriteString("UNKNOWN retains `stage`, `step`, `reason`, `unknown_class`, `next_operation`, and `blocked_by`. REFUTED is reserved for contradictory or malformed conditions.\n\n")
	for _, result := range results {
		if result.Unknown != nil {
			fmt.Fprintf(&builder, "- `%s` UNKNOWN: `%s/%s`, class `%s`, next `%s`, blocked by `%s`.\n", result.CaseID, result.Unknown.Stage, result.Unknown.Step, result.Unknown.UnknownClass, result.Unknown.NextOperation, strings.Join(result.Unknown.BlockedBy, ","))
		}
		for _, witness := range result.ConflictWitnesses {
			fmt.Fprintf(&builder, "- `%s` witness `%s`: `%s` vs `%s`, resource `%s`, evidence `%s`.\n", result.CaseID, witness.RuleID, witness.LeftProposal, witness.RightProposal, witness.Resource, strings.Join(witness.Evidence, ";"))
		}
	}
	builder.WriteString("\n")

	builder.WriteString("## Deterministic replay\n\n")
	fmt.Fprintf(&builder, "Normal projection digest: `%s`\n\nOrder-perturbed projection digest: `%s`\n\nReplay state: `%s` (`%s`); immutable receipt: `%t`. Review evidence: `%s`.\n\n", replay.NormalDigest, replay.OrderPerturbedDigest, replay.State, replay.Reason, replay.Immutable, strings.Join(provenance.Evidence, ";"))
	builder.WriteString("## Generated artifacts\n\n")
	for _, name := range RequiredArtifactNames {
		fmt.Fprintf(&builder, "- `%s`\n", name)
	}
	builder.WriteString("\nAll artifacts are caller-owned output. The source repository remains read-only.\n")
	return builder.String()
}
