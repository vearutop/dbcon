package dbcon_test

import (
	"os/exec"
	"testing"
)

// TestCompleterJS runs the contextual completer checks, if node is available.
func TestCompleterJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available")
	}

	if out, err := exec.Command(node, "testdata/completer_test.js").CombinedOutput(); err != nil { //nolint:gosec
		t.Fatalf("%v\n%s", err, out)
	}
}
