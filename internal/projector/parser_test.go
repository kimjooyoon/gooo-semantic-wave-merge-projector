package projector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGraphRejectsMissingRuntimeRule(t *testing.T) {
	root := repositoryRoot(t)
	source := filepath.Join(root, ".gooo", "semantic-wave-merge-projector.gooo")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	const declaration = "rule id=missing_evidence invariant=required-evidence-closed state=UNKNOWN reason=REQUIRED_EVIDENCE_DIGEST_NOT_AVAILABLE unknown_class=MISSING_EVIDENCE next_operation=SUPPLY_VERIFIED_EVIDENCE_DIGESTS"
	if !strings.Contains(string(raw), declaration) {
		t.Fatal("fixture does not contain the missing-evidence rule declaration")
	}
	mutated := strings.Replace(string(raw), declaration, "", 1)
	if _, err := parseGraph(source, []byte(mutated)); err == nil || !strings.Contains(err.Error(), "missing runtime rule missing_evidence") {
		t.Fatalf("parseGraph error = %v, want missing runtime rule", err)
	}
}
