package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kimjooyoon/gooo-semantic-wave-merge-projector/internal/projector"
)

func main() {
	if len(os.Args) < 2 {
		fatal("command is required: check, generate, or conformance")
	}
	switch os.Args[1] {
	case "check":
		check(os.Args[2:])
	case "generate":
		generate(os.Args[2:], false)
	case "conformance":
		generate(os.Args[2:], true)
	default:
		fatal("unknown command %q", os.Args[1])
	}
}

type flags struct {
	source, cases, output, root string
}

func parseFlags(command string, args []string, outputRequired bool) flags {
	set := flag.NewFlagSet(command, flag.ExitOnError)
	values := flags{}
	set.StringVar(&values.source, "source", ".gooo/semantic-wave-merge-projector.gooo", "released .gooo semantic graph")
	set.StringVar(&values.cases, "cases", "fixtures/cases", "canonical proposal fixture directory")
	set.StringVar(&values.output, "output", "", "absolute empty caller-owned output directory")
	set.StringVar(&values.root, "root", ".", "source repository root")
	set.Parse(args)
	if outputRequired && values.output == "" {
		fatal("%s requires --output", command)
	}
	return values
}

func check(args []string) {
	values := parseFlags("check", args, false)
	ir, _, err := projector.LoadGraph(values.source)
	if err != nil {
		fatal(err.Error())
	}
	fixtures, err := projector.LoadCases(values.cases, ir.Graph)
	if err != nil {
		fatal(err.Error())
	}
	printJSON(struct {
		Schema                 string `json:"schema"`
		Invariants             int    `json:"invariants"`
		ProposalFields         int    `json:"proposal_fields"`
		Cases                  int    `json:"cases"`
		Artifacts              int    `json:"artifacts"`
		RepositoryWrites       int    `json:"repository_writes"`
		LocalTestExecutions    int    `json:"local_test_executions"`
		CrossProjectGates      int    `json:"cross_project_required_gates"`
	}{ir.Schema, len(ir.Graph.Invariants), len(ir.Graph.Fields), len(fixtures), len(ir.Graph.Artifacts), ir.Graph.RepositoryWrites, ir.Graph.LocalTestExecutions, ir.Graph.CrossProjectRequiredGates})
}

func generate(args []string, conformance bool) {
	values := parseFlags("generate", args, true)
	result, err := projector.Generate(values.source, values.cases, values.output, values.root)
	if err != nil {
		fatal(err.Error())
	}
	if conformance {
		if err := verifyConformance(values.output, result); err != nil {
			fatal(err.Error())
		}
	}
	printJSON(struct {
		Decision            string `json:"decision"`
		ScenarioDenominator int    `json:"scenario_denominator"`
		Closed              int    `json:"closed"`
		Unknown             int    `json:"unknown"`
		Refuted             int    `json:"refuted"`
		ReplayMatch         bool   `json:"replay_match"`
		OutputDirectory     string `json:"output_directory"`
	}{"CONFORMANT", result.Denominator.ScenarioDenominator, result.Denominator.StateCounts.Closed, result.Denominator.StateCounts.Unknown, result.Denominator.StateCounts.Refuted, result.Replay.Match, filepath.Clean(values.output)})
}

func verifyConformance(output string, result projector.GenerationResult) error {
	if result.Denominator.ScenarioDenominator != 12 {
		return fmt.Errorf("scenario denominator is %d, expected 12", result.Denominator.ScenarioDenominator)
	}
	if result.Denominator.StateCounts != result.Denominator.ExpectedStateCounts || result.Denominator.StateCounts != (projector.StateCounts{Total: 12, Closed: 4, Unknown: 4, Refuted: 4}) {
		return fmt.Errorf("state distribution is %+v, expected four CLOSED, four UNKNOWN, four REFUTED", result.Denominator.StateCounts)
	}
	if !result.Replay.Match || !result.Replay.Immutable {
		return fmt.Errorf("deterministic immutable replay receipt failed")
	}
	for _, value := range result.Denominator.ProofVector {
		if value.Count != 4 {
			return fmt.Errorf("proof vector entry %s has count %d", value.Label, value.Count)
		}
	}
	for _, value := range result.Denominator.IndicatorVector {
		if value.Count != 4 {
			return fmt.Errorf("indicator vector entry %s has count %d", value.Label, value.Count)
		}
	}
	for _, assertion := range result.Assertions {
		if !assertion.Pass {
			return fmt.Errorf("assertion failed: %s", assertion.Name)
		}
	}
	for _, value := range result.Denominator.Cases {
		if value.State == projector.StateUnknown && !value.Unknown.Valid() {
			return fmt.Errorf("UNKNOWN case %s does not contain all six fields", value.CaseID)
		}
		for _, proposal := range value.Proposals {
			if proposal.State == projector.StateUnknown && !proposal.Unknown.Valid() {
				return fmt.Errorf("UNKNOWN proposal %s does not contain all six fields", proposal.ProposalID)
			}
		}
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		return err
	}
	if len(entries) != len(projector.RequiredArtifactNames) {
		return fmt.Errorf("conformance output has %d files, expected %d", len(entries), len(projector.RequiredArtifactNames))
	}
	for _, name := range projector.RequiredArtifactNames {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			return fmt.Errorf("missing artifact %s: %w", name, err)
		}
	}
	return nil
}

func printJSON(value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		fatal(err.Error())
	}
	fmt.Println(string(raw))
}

func fatal(format string, values ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}
