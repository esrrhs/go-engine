package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/esrrhs/divvy/pkg/agent"
	"github.com/esrrhs/divvy/pkg/cost"
	"github.com/esrrhs/divvy/pkg/llm"
	"github.com/esrrhs/divvy/pkg/models"
)

// benchConfig is one bench invocation's settings, shared by every goal run.
type benchConfig struct {
	BaseURL   string
	Model     string
	APIKey    string
	ExtraJSON string

	MaxSteps   int
	MaxRetries int
	Parallel   int
	Isolate    bool
	GitCommit  bool
	Timeout    time.Duration

	// Root is the directory that holds one subdirectory per goal run (the
	// seeded workspace plus the run's own .divvy session data). Workspaces
	// are kept after the run for post-mortem inspection.
	Root string

	Pricing   *cost.Pricing
	newClient func(agent.Config) llm.Client // nil builds the real client
}

// RunResult is one goal run's outcome and cost accounting.
type RunResult struct {
	Goal   string `json:"goal"`
	Repeat int    `json:"repeat"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`

	WallMS int64 `json:"wall_ms"`

	LLMCalls         int     `json:"llm_calls"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`

	Leaves     int `json:"leaves"`
	LeavesDone int `json:"leaves_done"`

	VerifyFail int `json:"verify_fail"`
	Retries    int `json:"retries"`
	Stalls     int `json:"stalls"`
	Resplits   int `json:"resplits"`
	GiveUps    int `json:"give_ups"`

	Workdir   string `json:"workdir"`
	SessionID string `json:"session_id"`
}

// Aggregate rolls results up for the bench-level verdict.
type Aggregate struct {
	Runs        int     `json:"runs"`
	OK          int     `json:"ok"`
	WallMS      int64   `json:"wall_ms"`
	LLMCalls    int     `json:"llm_calls"`
	TotalTokens int     `json:"total_tokens"`
	CostUSD     float64 `json:"cost_usd"`
	VerifyFail  int     `json:"verify_fail"`
	Retries     int     `json:"retries"`
	Stalls      int     `json:"stalls"`
	Resplits    int     `json:"resplits"`
	SuccessRate float64 `json:"success_rate"`
}

func aggregate(rs []RunResult) Aggregate {
	var a Aggregate
	for _, r := range rs {
		a.Runs++
		if r.OK {
			a.OK++
		}
		a.WallMS += r.WallMS
		a.LLMCalls += r.LLMCalls
		a.TotalTokens += r.TotalTokens
		a.CostUSD += r.CostUSD
		a.VerifyFail += r.VerifyFail
		a.Retries += r.Retries
		a.Stalls += r.Stalls
		a.Resplits += r.Resplits
	}
	if a.Runs > 0 {
		a.SuccessRate = float64(a.OK) / float64(a.Runs)
	}
	return a
}

// seedWorkspace materializes the goal's seed files, initializes the Go
// module, and makes an initial git commit so -git-commit has a baseline and
// a post-mortem `git diff` shows exactly what the model changed.
func (g Goal) seedWorkspace(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for rel, content := range g.Seed {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return fmt.Errorf("seed %s: %w", rel, err)
		}
	}
	if g.Module != "" {
		if out, err := runCmd(dir, "go", "mod", "init", g.Module); err != nil {
			return fmt.Errorf("go mod init: %v: %s", err, out)
		}
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "bench@divvy.local"},
		{"config", "user.name", "divvy-bench"},
		{"add", "-A"},
		{"commit", "-q", "-m", "seed"},
	} {
		if out, err := runCmd(dir, "git", args...); err != nil {
			return fmt.Errorf("git %v: %v: %s", args, err, out)
		}
	}
	return nil
}

func runCmd(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runGoal executes one goal once in a fresh workspace and collects the
// outcome. It never panics on run failure: a failed goal is a data point,
// not a harness error.
func runGoal(ctx context.Context, g Goal, repeat int, bc benchConfig) RunResult {
	res := RunResult{Goal: g.Name, Repeat: repeat}
	res.Workdir = filepath.Join(bc.Root, fmt.Sprintf("%s-r%d", g.Name, repeat))

	if err := g.seedWorkspace(res.Workdir); err != nil {
		res.Error = "seed workspace: " + err.Error()
		return res
	}

	cfg := agent.DefaultConfig()
	cfg.Goal = g.Prompt
	cfg.WorkDir = res.Workdir
	cfg.DataDir = filepath.Join(res.Workdir, ".divvy")
	cfg.Model = bc.Model
	cfg.BaseURL = bc.BaseURL
	cfg.APIKey = bc.APIKey
	cfg.ExtraJSON = bc.ExtraJSON
	cfg.Stream = false // token streaming adds noise, not signal, to a bench
	if bc.MaxSteps > 0 {
		cfg.MaxSteps = bc.MaxSteps
	}
	if bc.MaxRetries > 0 {
		cfg.MaxRetries = bc.MaxRetries
	}
	if bc.Parallel > 0 {
		cfg.Parallel = bc.Parallel
	}
	cfg.Isolate = bc.Isolate
	cfg.GitCommit = bc.GitCommit

	newClient := bc.newClient
	if newClient == nil {
		newClient = func(cfg agent.Config) llm.Client {
			c := llm.NewOpenAIClient(cfg.APIKey, cfg.BaseURL, cfg.RequestTimeout)
			c.ExtraJSON = cfg.ExtraJSON
			c.MaxBackoff = cfg.RetryMaxInterval
			return c
		}
	}

	o, err := agent.NewFromGoal(cfg, newClient(cfg), agent.SilentLogger())
	if err != nil {
		res.Error = "create orchestrator: " + err.Error()
		return res
	}
	defer o.Close()
	res.SessionID = o.SessionID()

	runCtx, cancel := context.WithTimeout(ctx, bc.Timeout)
	defer cancel()

	start := time.Now()
	runErr := o.Run(runCtx)
	res.WallMS = time.Since(start).Milliseconds()

	usage := o.Tree().TotalTokenUsage()
	res.LLMCalls = usage.Calls
	res.PromptTokens = usage.PromptTokens
	res.CompletionTokens = usage.CompletionTokens
	res.TotalTokens = usage.TotalTokens
	if bc.Pricing != nil {
		if p, ok := bc.Pricing.PriceFor(bc.Model); ok && !p.Zero() {
			res.CostUSD = p.Cost(usage.PromptTokens, usage.CompletionTokens)
		}
	}
	res.LeavesDone, res.Leaves, _ = o.Tree().GetLeafProgress()
	if summary, serr := agent.SummarizeEvents(o.EventPath()); serr == nil {
		res.VerifyFail = summary.VerifyFail
		res.Retries = summary.Retries
		res.Stalls = summary.Stalls
		res.Resplits = summary.Resplits
		res.GiveUps = summary.GiveUps
	}

	switch {
	case runErr != nil:
		res.Error = oneLine(runErr.Error())
	case rootCompleted(o):
		res.OK = true
	default:
		res.Error = "run returned no error but the root goal is not COMPLETED"
	}
	return res
}

// rootCompleted reports the goal-level verdict, mirroring
// engine.Scheduler.IsComplete: the root must be COMPLETED and, for a
// compound root, its goal-level acceptance DoD must have passed
// (IntegrationVerified). For an atomic goal the root IS a leaf and its own
// DoD is the acceptance, so COMPLETED alone suffices — do not require
// IntegrationVerified there, it is deliberately never set for leaf roots.
func rootCompleted(o *agent.Orchestrator) bool {
	root := o.Tree().GetRoot()
	if root == nil || root.State != models.TaskStateCompleted {
		return false
	}
	return root.Type == models.NodeTypeLeaf || root.IntegrationVerified
}

// oneLine collapses an error message onto a single bounded line.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 90 {
		s = s[:90] + "..."
	}
	return s
}
