package tools

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestRunBash_MissingRootNoPanic guards the deferred process-group cleanup:
// when cmd.Start fails (here: the working directory was removed), cmd.Process
// is nil and RunBash must surface the launch error rather than panicking in
// the cleanup closure. Observed under full -race load when a leaf's mirror
// was gone before its verify command launched.
func TestRunBash_MissingRootNoPanic(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	sb := &Sandbox{Root: missing}

	res, err := sb.RunBash(context.Background(), "true", time.Second)
	if err == nil {
		t.Fatalf("expected a start error for missing root, got res=%+v", res)
	}
}
