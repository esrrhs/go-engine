package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/esrrhs/divvy/pkg/agent"
	"github.com/esrrhs/divvy/pkg/engine"
	"github.com/esrrhs/divvy/pkg/models"
)

// NFR-5 scale: a finished session with a 200-leaf tree and 10k persisted
// events. The SSE reconnect path must stay bounded — replay streams only
// the 200-event tail plus one tree snapshot regardless of file size.

const (
	scaleSessionID = "sess_scale"
	scaleEvents    = 10_000
	scaleReplay    = 200
)

func buildScaleFixture(t *testing.T, dataDir string) {
	t.Helper()

	// 201 nodes: root + 10 compound + 200 leaves, all populated like a real run.
	tree := engine.NewTaskTree(scaleSessionID, "200 叶子压力会话", "NFR-5 规模验证")
	now := time.Now()
	for c := 0; c < 10; c++ {
		cid := fmt.Sprintf("c%02d", c)
		if _, err := tree.AddChild("root", cid, fmt.Sprintf("子系统 %d", c), "复合任务", models.NodeTypeCompound); err != nil {
			t.Fatal(err)
		}
		for l := 0; l < 20; l++ {
			lid := fmt.Sprintf("%s_l%02d", cid, l)
			if _, err := tree.AddChild(cid, lid, fmt.Sprintf("叶子 %d-%d", c, l), "原子任务", models.NodeTypeLeaf); err != nil {
				t.Fatal(err)
			}
			states := []models.TaskState{models.TaskStateCompleted, models.TaskStateRunning, models.TaskStatePending}
			if err := tree.UpdateNode(lid, func(n *models.TaskNode) error {
				n.State = states[(c+l)%3]
				n.Contract.Outputs = []string{fmt.Sprintf("out/%s.txt", lid)}
				n.DoD.Commands = []string{"go test ./..."}
				n.TokenUsage = models.TokenUsage{Calls: 3, TotalTokens: 1040}
				n.ResultSummary = "按契约完成"
				n.ErrorHistory = []models.ErrorRecord{{Time: now, Error: "first attempt failed", Fingerprint: "fp"}}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	stor, err := engine.NewStorage(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := stor.SaveTree(tree); err != nil {
		t.Fatal(err)
	}

	// 10k JSONL events, a few hundred bytes each.
	evPath := filepath.Join(dataDir, "events", scaleSessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(evPath), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(evPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := 1; i <= scaleEvents; i++ {
		ev := map[string]any{
			"seq":     i,
			"ts":      "2026-10-10T12:00:00+08:00",
			"node_id": fmt.Sprintf("c%02d_l%02d", (i/200)%10, i%20),
			"kind":    [3]string{"state_change", "llm_call", "tool_call"}[i%3],
			"to":      "RUNNING",
			"model":   "mock-model",
			"detail":  fmt.Sprintf("event %05d with padding padding padding padding", i),
		}
		if err := enc.Encode(ev); err != nil {
			t.Fatal(err)
		}
	}
}

// TestScaleSSEReplay10k: reconnecting to a 10k-event finished session
// delivers exactly the last 200 events + one snapshot, quickly.
func TestScaleSSEReplay10k(t *testing.T) {
	dataDir := t.TempDir()
	buildScaleFixture(t, dataDir)
	info, _ := os.Stat(filepath.Join(dataDir, "events", scaleSessionID+".jsonl"))
	t.Logf("event file: %.1f MiB", float64(info.Size())/1024/1024)

	ts := startSSEServer(t, agent.NewRunManager(), dataDir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/api/sessions/"+scaleSessionID+"/events?token=test-token", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE status=%d", resp.StatusCode)
	}

	start := time.Now()
	r := bufio.NewReader(resp.Body)
	replayed := 0
	snapshots := 0
	var minSeq, maxSeq int64 = 1 << 62, 0
	deadline := time.Now().Add(3 * time.Second)
	for snapshots == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("timeout: replayed=%d snapshots=%d", replayed, snapshots)
		}
		f, err := readFrame(r)
		if err != nil {
			t.Fatalf("read frame: %v (replayed=%d)", err, replayed)
		}
		if f.data == "" {
			continue // heartbeat comment
		}
		var head struct {
			Kind string `json:"kind"`
			Seq  int64  `json:"seq"`
		}
		if err := json.Unmarshal([]byte(f.data), &head); err != nil {
			t.Fatalf("frame json: %v", err)
		}
		if head.Kind == "tree_snapshot" {
			snapshots++
			continue
		}
		replayed++
		if head.Seq < minSeq {
			minSeq = head.Seq
		}
		if head.Seq > maxSeq {
			maxSeq = head.Seq
		}
	}
	elapsed := time.Since(start)

	if replayed != scaleReplay {
		t.Fatalf("replayed=%d, want exactly %d (tail must be bounded)", replayed, scaleReplay)
	}
	if snapshots != 1 {
		t.Fatalf("snapshots=%d, want 1", snapshots)
	}
	if minSeq != scaleEvents-scaleReplay+1 || maxSeq != scaleEvents {
		t.Fatalf("replayed seq window=[%d,%d], want [%d,%d]",
			minSeq, maxSeq, scaleEvents-scaleReplay+1, scaleEvents)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("replay of tail over 10k events took %s, want <2s", elapsed)
	}
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	t.Logf("replay %d frames + snapshot in %s, heap after GC %.1f MiB",
		replayed, elapsed, float64(ms.HeapAlloc)/1024/1024)
}

// TestScaleDetailAndList: fetching the big tree and the session index both
// succeed and the detail carries all 200 leaves.
func TestScaleDetailAndList(t *testing.T) {
	dataDir := t.TempDir()
	buildScaleFixture(t, dataDir)
	ts := startSSEServer(t, agent.NewRunManager(), dataDir)

	resp, err := http.Get(ts.URL + "/api/sessions/" + scaleSessionID + "?token=test-token")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", resp.StatusCode, raw)
	}
	var detail struct {
		Tree json.RawMessage `json:"tree"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Tree) < 100_000 {
		t.Fatalf("detail tree too small: %d bytes", len(detail.Tree))
	}
	t.Logf("detail tree JSON %.1f KiB", float64(len(detail.Tree))/1024)

	lresp, err := http.Get(ts.URL + "/api/sessions?token=test-token")
	if err != nil {
		t.Fatal(err)
	}
	lresp.Body.Close()
	if lresp.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d", lresp.StatusCode)
	}
}
