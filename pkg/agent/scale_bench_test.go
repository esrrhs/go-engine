package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeScaleEventFile lays down n realistically sized JSONL events
// (state_change / llm_call / tool_call, each a few hundred bytes) and
// returns the path. This is the NFR-5 10k-event stress fixture.
func writeScaleEventFile(b testing.TB, dir string, n int) string {
	b.Helper()
	path := filepath.Join(dir, "events", "sess_scale.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := 1; i <= n; i++ {
		ev := map[string]any{
			"seq":       i,
			"ts":        "2026-10-10T12:00:00+08:00",
			"node_id":   fmt.Sprintf("c%02d_l%02d", (i/200)%10, i%20),
			"to":        "RUNNING",
			"model":     "mock-model",
			"call_kind": "worker",
		}
		switch i % 3 {
		case 0:
			ev["kind"] = "state_change"
		case 1:
			ev["kind"] = "llm_call"
			ev["output_preview"] = "go test ./... ok\n" + fmt.Sprintf("padding line %03d", i)
		case 2:
			ev["kind"] = "tool_call"
			ev["n_messages"] = 12
		}
		if err := enc.Encode(ev); err != nil {
			b.Fatal(err)
		}
	}
	return path
}

// Opening a 10k-event file on resume must stay cheap: seq continuation
// scans only the final 1 MiB, and the torn-line trim inspects 64 KiB.
func BenchmarkNewEventRecorder10k(b *testing.B) {
	dir := b.TempDir()
	path := writeScaleEventFile(b, dir, 10_000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := NewEventRecorder(path)
		if err != nil {
			b.Fatal(err)
		}
		if r.seq != 10_000 {
			b.Fatalf("highest seq=%d, want 10000", r.seq)
		}
		if err := r.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// TestScaleEventSeq10k asserts seq continuation at the 10k-event scale and
// that the next Record appends seq 10001 rather than colliding.
func TestScaleEventSeq10k(t *testing.T) {
	dir := t.TempDir()
	path := writeScaleEventFile(t, dir, 10_000)
	info, _ := os.Stat(path)
	t.Logf("event file: %.1f MiB", float64(info.Size())/1024/1024)

	r, err := NewEventRecorder(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.seq != 10_000 {
		t.Fatalf("resume seq=%d, want 10000", r.seq)
	}
	r.Record("state_change", "root", map[string]any{"to": "COMPLETED"})
	if r.seq != 10_001 {
		t.Fatalf("next seq=%d, want 10001", r.seq)
	}
	if got := highestEventSeq(path); got != 10_001 {
		t.Fatalf("persisted highest seq=%d, want 10001", got)
	}
}
