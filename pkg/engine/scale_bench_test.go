package engine

import (
	"fmt"
	"testing"
	"time"

	"github.com/esrrhs/divvy/pkg/models"
)

// buildScaleTree builds the NFR-5 stress shape: one root, nCompound
// compound children, each with nLeaves leaves — 201 nodes at the spec's
// 200-leaf mark. Every node carries realistic metadata (contract, DoD,
// token usage, a retry with an error record) so JSON size reflects a real
// run rather than empty stubs.
func buildScaleTree(nCompound, nLeaves int) *TaskTree {
	tree := NewTaskTree("sess_scale", "200 leaf stress run", "verify the UI at NFR-5 scale")
	now := time.Now()
	for c := 0; c < nCompound; c++ {
		cid := fmt.Sprintf("c%02d", c)
		if _, err := tree.AddChild("root", cid,
			fmt.Sprintf("子系统 %d：一组相关叶子任务", c),
			"每个复合节点聚合约二十个叶子，覆盖增删改查与验收", models.NodeTypeCompound); err != nil {
			panic(err)
		}
		for l := 0; l < nLeaves; l++ {
			lid := fmt.Sprintf("%s_l%02d", cid, l)
			if _, err := tree.AddChild(cid, lid,
				fmt.Sprintf("叶子 %d-%d：实现并验证一个具体改动", c, l),
				"弱模型在隔离镜像中完成文件改动并通过 DoD 命令", models.NodeTypeLeaf); err != nil {
				panic(err)
			}
			states := []models.TaskState{
				models.TaskStateCompleted, models.TaskStateRunning, models.TaskStatePending,
			}
			if err := tree.UpdateNode(lid, func(n *models.TaskNode) error {
				n.State = states[(c+l)%len(states)]
				n.Contract.Inputs = []string{"spec.md", "existing_code.go"}
				n.Contract.Outputs = []string{fmt.Sprintf("out/%s.txt", lid)}
				n.Contract.Constraints = []string{"不破坏现有 API", "保持向后兼容"}
				n.DoD.Commands = []string{"go test ./...", fmt.Sprintf("test -f out/%s.txt", lid)}
				n.DoD.TimeoutSec = 60
				n.TokenUsage = models.TokenUsage{Calls: 3, PromptTokens: 800, CompletionTokens: 240, TotalTokens: 1040}
				n.ResultSummary = "按契约产出文件并通过验收命令"
				n.RetryCount = 1
				n.ErrorHistory = []models.ErrorRecord{{
					Time:        now.Add(-time.Minute),
					Error:       "verify failed: exit status 1 — expected file missing on first attempt",
					Fingerprint: "fp_" + lid,
				}}
				n.CreatedAt = now
				n.UpdatedAt = now
				return nil
			}); err != nil {
				panic(err)
			}
		}
		if err := tree.UpdateNode(cid, func(n *models.TaskNode) error {
			n.State = models.TaskStateCompleted
			n.IntegrationVerified = true
			return nil
		}); err != nil {
			panic(err)
		}
	}
	return tree
}

// BenchmarkTreeToJSON200 measures serializing the 200-leaf tree that every
// coalesced state change may publish to SSE subscribers.
func BenchmarkTreeToJSON200(b *testing.B) {
	tree := buildScaleTree(10, 20)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := tree.ToJSON(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTreeFromJSON200 measures the detail endpoint's load path.
func BenchmarkTreeFromJSON200(b *testing.B) {
	tree := buildScaleTree(10, 20)
	data, err := tree.ToJSON()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := FromJSON(data); err != nil {
			b.Fatal(err)
		}
	}
}

// TestScaleTreeShape sanity-checks the fixture itself.
func TestScaleTreeShape(t *testing.T) {
	tree := buildScaleTree(10, 20)
	done, total, _ := tree.GetLeafProgress()
	if total != 200 {
		t.Fatalf("leaves=%d, want 200", total)
	}
	if done <= 0 || done >= 200 {
		t.Fatalf("mixed states expected, done=%d", done)
	}
	data, err := tree.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100_000 {
		t.Fatalf("200-leaf tree JSON suspiciously small: %d bytes", len(data))
	}
	t.Logf("scale tree: %d nodes, JSON %.1f KiB", 201, float64(len(data))/1024)
}
