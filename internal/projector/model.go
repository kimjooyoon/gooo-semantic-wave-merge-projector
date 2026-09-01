package projector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

const (
	GraphSchema        = "gooo/semantic-wave-merge-projector/semantic-graph/v1"
	IRSchema           = "gooo/semantic-wave-merge-projector/semantic-ir/v1"
	ProjectionSchema   = "gooo/semantic-wave-merge-projector/wave-projection/v1"
	DistributionSchema = "gooo/semantic-wave-merge-projector/wave-distribution/v1"
	AssertionSchema    = "gooo/semantic-wave-merge-projector/generated-assertions/v1"
	EventSchema        = "gooo/semantic-wave-merge-projector/projection-event/v1"
	ReplaySchema       = "gooo/semantic-wave-merge-projector/replay-receipt/v1"
	StateClosed        = "CLOSED"
	StateUnknown       = "UNKNOWN"
	StateRefuted       = "REFUTED"
	ResultMergeable    = "MERGEABLE(CLOSED)"
	ToolchainVersion   = "go1.27.0"
	RunnerIdentity     = "github-actions/ubuntu-latest"
)

var RequiredArtifactNames = []string{
	"wave-projection.json",
	"wave-distribution.json",
	"generated-assertions.json",
	"projection-events.ndjson",
	"replay-receipt.json",
	"report.md",
}

var RequiredProposalFields = []string{
	"proposal_id",
	"base_ledger_digest",
	"semantic_read_set",
	"semantic_write_set",
	"required_evidence_digests",
	"tool_release_locks",
	"causal_dependencies",
	"authority_scope",
}

type SourceLocation struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type ArtifactDecl struct {
	Ordinal int            `json:"ordinal"`
	Name    string         `json:"name"`
	Source  SourceLocation `json:"source_location"`
}

type FieldDecl struct {
	Ordinal  int            `json:"ordinal"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Required bool           `json:"required"`
	Source   SourceLocation `json:"source_location"`
}

type StateDecl struct {
	Ordinal       int            `json:"ordinal"`
	Name          string         `json:"name"`
	Result        string         `json:"result"`
	NextOperation string         `json:"next_operation"`
	Source        SourceLocation `json:"source_location"`
}

type AuthorityDecl struct {
	Ordinal int            `json:"ordinal"`
	Name    string         `json:"name"`
	Scope   []string       `json:"scope"`
	Source  SourceLocation `json:"source_location"`
}

type BootstrapProvenanceDecl struct {
	ID     string         `json:"id"`
	State  string         `json:"state"`
	Reason string         `json:"reason"`
	Commit string         `json:"commit"`
	Ref    string         `json:"ref"`
	Source SourceLocation `json:"source_location"`
}

type ReviewGateDecl struct {
	ID            string         `json:"id"`
	Required      bool           `json:"required"`
	MissingState  string         `json:"missing_state"`
	ReviewedState string         `json:"reviewed_state"`
	MergedState   string         `json:"merged_state"`
	Reason        string         `json:"reason"`
	UnknownClass  string         `json:"unknown_class"`
	NextOperation string         `json:"next_operation"`
	Source        SourceLocation `json:"source_location"`
}

type InvariantDecl struct {
	Ordinal   int            `json:"ordinal"`
	StableID  string         `json:"stable_id"`
	Class     string         `json:"class"`
	Proof     string         `json:"proof_choice"`
	Indicator string         `json:"indicator_class"`
	Stage     string         `json:"stage"`
	Step      string         `json:"step"`
	DependsOn []string       `json:"depends_on"`
	Source    SourceLocation `json:"source_location"`
}

type RuleDecl struct {
	StableID      string         `json:"stable_id"`
	Invariant     string         `json:"invariant"`
	State         string         `json:"state"`
	Reason        string         `json:"reason"`
	UnknownClass  string         `json:"unknown_class"`
	NextOperation string         `json:"next_operation"`
	Source        SourceLocation `json:"source_location"`
}

type CaseContract struct {
	Ordinal  int            `json:"ordinal"`
	StableID string         `json:"stable_id"`
	Expected string         `json:"expected_state"`
	Fixture  string         `json:"fixture"`
	Source   SourceLocation `json:"source_location"`
}

type SemanticGraph struct {
	Schema                    string                     `json:"schema"`
	GraphID                   string                     `json:"graph_id"`
	Release                   string                     `json:"release"`
	Precedence                []string                   `json:"precedence"`
	RepositoryWrites          int                        `json:"repository_writes"`
	LocalTestExecutions       int                        `json:"local_test_executions"`
	CrossProjectRequiredGates int                        `json:"cross_project_required_gates"`
	OutputArtifactCount       int                        `json:"output_artifact_count"`
	LedgerDigest              string                     `json:"ledger_digest"`
	Artifacts                 []ArtifactDecl             `json:"artifacts"`
	Fields                    []FieldDecl                `json:"fields"`
	States                    map[string]StateDecl       `json:"states"`
	Authorities               []AuthorityDecl            `json:"authorities"`
	BootstrapProvenance       BootstrapProvenanceDecl    `json:"bootstrap_provenance"`
	ReviewGate                ReviewGateDecl             `json:"review_gate"`
	ReviewFixture             string                     `json:"review_fixture"`
	Invariants                []InvariantDecl            `json:"invariants"`
	Rules                     map[string]RuleDecl        `json:"rules"`
	Cases                     []CaseContract             `json:"cases"`
}

type SemanticIR struct {
	Schema       string        `json:"schema"`
	SourcePath   string        `json:"source_path"`
	SourceDigest string        `json:"source_digest"`
	Graph        SemanticGraph `json:"graph"`
}

type ToolReleaseLockInput struct {
	ToolID        string `json:"tool_id"`
	ReleaseDigest string `json:"release_digest"`
	Mutable       *bool  `json:"mutable"`
	Verified      *bool  `json:"verified"`
}

type ProposalInput struct {
	ProposalID         string                 `json:"proposal_id"`
	BaseLedgerDigest   string                 `json:"base_ledger_digest"`
	SemanticReadSet    []string               `json:"semantic_read_set"`
	SemanticWriteSet   []string               `json:"semantic_write_set"`
	RequiredEvidence   []string               `json:"required_evidence_digests"`
	ToolReleaseLocks   []ToolReleaseLockInput `json:"tool_release_locks"`
	CausalDependencies []string               `json:"causal_dependencies"`
	AuthorityScope     []string               `json:"authority_scope"`
}

type FixtureInput struct {
	FixtureID        string          `json:"fixture_id"`
	ExpectedState    string          `json:"expected_state"`
	BaseLedgerDigest string          `json:"base_ledger_digest"`
	EvidenceDigests  []string        `json:"evidence_digests"`
	Proposals        []ProposalInput `json:"proposals"`
	FixturePath      string          `json:"-"`
}

type ReviewOptions struct {
	PullRequestNumber int
	MergeSHA          string
	ReleaseTag        string
}

type ReviewFixture struct {
	FixtureID          string `json:"fixture_id"`
	Kind               string `json:"kind"`
	Required           bool   `json:"required"`
	BootstrapState     string `json:"bootstrap_state"`
	BootstrapReason    string `json:"bootstrap_reason"`
	MissingReviewState string `json:"missing_review_state"`
	ReviewedState      string `json:"reviewed_state"`
	MergedState        string `json:"merged_state"`
	EvidenceSource     string `json:"evidence_source"`
	FailClosed         bool   `json:"fail_closed"`
}

type OperatorProvenanceReceipt struct {
	BootstrapState    string        `json:"bootstrap_state"`
	BootstrapReason   string        `json:"bootstrap_reason"`
	BootstrapCommit   string        `json:"bootstrap_commit"`
	BootstrapRef      string        `json:"bootstrap_ref"`
	ReviewGate        string        `json:"review_gate"`
	PullRequestNumber int           `json:"pull_request_number"`
	MergeSHA         string        `json:"merge_sha,omitempty"`
	ReleaseTag       string        `json:"release_tag,omitempty"`
	Evidence         []string      `json:"evidence"`
	Unknown          *UnknownClaim `json:"unknown,omitempty"`
}

type UnknownClaim struct {
	Stage         string   `json:"stage"`
	Step          string   `json:"step"`
	Reason        string   `json:"reason"`
	UnknownClass  string   `json:"unknown_class"`
	NextOperation string   `json:"next_operation"`
	BlockedBy     []string `json:"blocked_by"`
}

func (u *UnknownClaim) Valid() bool {
	return u != nil && u.Stage != "" && u.Step != "" && u.Reason != "" &&
		u.UnknownClass != "" && u.NextOperation != "" && u.BlockedBy != nil
}

type Issue struct {
	RuleID    string   `json:"rule_id"`
	State     string   `json:"state"`
	Reason    string   `json:"reason"`
	Evidence  []string `json:"evidence"`
	BlockedBy []string `json:"blocked_by"`
}

type ProposalProjection struct {
	ProposalID     string        `json:"proposal_id"`
	State          string        `json:"state"`
	Result         string        `json:"result"`
	Unknown        *UnknownClaim `json:"unknown,omitempty"`
	Issues         []Issue       `json:"issues"`
	Dependencies   []string      `json:"causal_dependencies"`
	ReadSet        []string      `json:"semantic_read_set"`
	WriteSet       []string      `json:"semantic_write_set"`
	AuthorityScope []string      `json:"authority_scope"`
}

type ConflictWitness struct {
	RuleID        string   `json:"rule_id"`
	State         string   `json:"state"`
	LeftProposal  string   `json:"left_proposal"`
	RightProposal string   `json:"right_proposal"`
	Resource      string   `json:"resource"`
	Evidence      []string `json:"evidence"`
}

type DeferredProposal struct {
	ProposalID string   `json:"proposal_id"`
	State      string   `json:"state"`
	Reason     string   `json:"reason"`
	BlockedBy  []string `json:"blocked_by"`
}

type CaseResult struct {
	Ordinal                       int                   `json:"ordinal"`
	InvariantID                   string                `json:"invariant_id"`
	CaseID                        string                `json:"case_id"`
	ExpectedState                 string                `json:"expected_state"`
	State                         string                `json:"state"`
	Result                        string                `json:"result"`
	Reason                        string                `json:"reason"`
	Unknown                       *UnknownClaim         `json:"unknown,omitempty"`
	BaseLedgerDigest              string                `json:"base_ledger_digest"`
	DeterministicTopologicalOrder []string              `json:"deterministic_topological_order"`
	TopologicalOrderValid         bool                  `json:"topological_order_valid"`
	AcceptedWave                  []string              `json:"accepted_wave"`
	DeferredFrontier              []DeferredProposal    `json:"deferred_frontier"`
	ConflictWitnesses             []ConflictWitness     `json:"conflict_witnesses"`
	Proposals                     []ProposalProjection  `json:"proposals"`
	NextOperation                 string                `json:"next_operation"`
	FixturePath                   string                `json:"fixture_path"`
}

type StateCounts struct {
	Total   int `json:"total"`
	Closed  int `json:"closed"`
	Unknown int `json:"unknown"`
	Refuted int `json:"refuted"`
}

type LabeledCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type VectorEntry struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type SemanticDenominator struct {
	Schema              string                    `json:"schema"`
	IRSchema            string                    `json:"ir_schema"`
	SourcePath          string                    `json:"source_path"`
	SourceDigest        string                    `json:"source_digest"`
	GraphID             string                    `json:"graph_id"`
	Release             string                    `json:"release"`
	ScenarioDenominator int                       `json:"scenario_denominator"`
	StateCounts         StateCounts               `json:"state_counts"`
	ExpectedStateCounts StateCounts               `json:"expected_state_counts"`
	Precedence          []string                  `json:"precedence"`
	ProofVector         []VectorEntry             `json:"proof_vector"`
	IndicatorVector     []VectorEntry             `json:"indicator_vector"`
	Invariants          []InvariantDecl           `json:"invariants"`
	Fields              []FieldDecl               `json:"proposal_fields"`
	Authority           map[string]any            `json:"authority"`
	OperatorProvenance  OperatorProvenanceReceipt `json:"operator_provenance"`
	OutputArtifacts     []string                  `json:"output_artifacts"`
	Cases               []CaseResult              `json:"cases"`
}

type SemanticDistribution struct {
	Schema           string        `json:"schema"`
	SourceDigest     string        `json:"source_digest"`
	States           StateCounts   `json:"states"`
	ProofVector      []VectorEntry `json:"proof_vector"`
	IndicatorVector  []VectorEntry `json:"indicator_vector"`
	DirectCountsOnly bool          `json:"direct_counts_only"`
}

type GeneratedAssertion struct {
	Name     string `json:"name"`
	Expected any    `json:"expected"`
	Observed any    `json:"observed"`
	Pass     bool   `json:"pass"`
}

type ProjectionEvent struct {
	Schema       string   `json:"schema"`
	Ordinal      int      `json:"ordinal"`
	CaseID       string   `json:"case_id"`
	State        string   `json:"state"`
	Result       string   `json:"result"`
	AcceptedWave []string `json:"accepted_wave"`
	Deferred     []string `json:"deferred_frontier"`
}

type ReplayReceipt struct {
	Schema                   string                    `json:"schema"`
	SourceDigest             string                    `json:"source_digest"`
	NormalInputOrder         []string                  `json:"normal_input_order"`
	OrderPerturbedInputOrder []string                  `json:"order_perturbed_input_order"`
	NormalDigest             string                    `json:"normal_digest"`
	OrderPerturbedDigest     string                    `json:"order_perturbed_digest"`
	Match                    bool                      `json:"match"`
	State                    string                    `json:"state"`
	Reason                   string                    `json:"reason"`
	Immutable                bool                      `json:"immutable"`
	OperatorProvenance       OperatorProvenanceReceipt `json:"operator_provenance"`
}

type GenerationResult struct {
	Denominator  SemanticDenominator
	Distribution SemanticDistribution
	Assertions   []GeneratedAssertion
	Events       []ProjectionEvent
	Replay       ReplayReceipt
	Report       string
}

func DigestBytes(raw []byte) string {
	digest := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func JSON(value any) ([]byte, error) {
	return json.Marshal(value)
}

func sortedCopy(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func sortStringsUnique(values []string) []string {
	result := sortedCopy(values)
	unique := result[:0]
	for _, value := range result {
		if len(unique) == 0 || unique[len(unique)-1] != value {
			unique = append(unique, value)
		}
	}
	return unique
}
