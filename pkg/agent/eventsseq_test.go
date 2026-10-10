package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// P2-5: a JSONL line exceeding the old Scanner's 16 MiB cap must not abort
// the seq scan. A tree_snapshot can be the oversized line; the higher-seq
// lines following it still have to win, or a resumed run mints colliding
// seq numbers.
func TestHighestEventSeq_OversizedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	write := func(ev map[string]any) {
		t.Helper()
		b, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	write(map[string]any{"seq": 1, "kind": "state_change"})
	// One valid JSON line larger than the old 16 MiB scanner ceiling.
	write(map[string]any{
		"seq":     2,
		"kind":    "tree_snapshot",
		"padding": strings.Repeat("x", 16*1024*1024+1024),
	})
	write(map[string]any{"seq": 3, "kind": "state_change"})
	write(map[string]any{"seq": 42, "kind": "session_end"})
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if got := highestEventSeq(path); got != 42 {
		t.Fatalf("highest seq across oversized line=%d, want 42", got)
	}
}

// P2-5: a torn (newline-less, unreadable) final line is skipped without
// aborting the scan, so the last complete line determines the highest seq.
func TestHighestEventSeq_TornTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	content := `{"seq":1,"kind":"state_change"}` + "\n" +
		`{"seq":7,"kind":"llm_call"}` + "\n" +
		`{"seq":8,"kind":"state_change"` // half-written, no terminating newline
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := highestEventSeq(path); got != 7 {
		t.Fatalf("highest seq with torn tail=%d, want 7", got)
	}
}
