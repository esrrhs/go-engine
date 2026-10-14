// Command divvy-bench is the weak-model benchmark harness for divvy.
//
// The engine's thesis is that engineering constraints, not model cleverness,
// carry complex goals on small models — but "it works on weak models" used to
// be backed only by scripted mock end-to-end tests. This harness makes the
// claim measurable: a fixed corpus of small, self-verifying goals is executed
// against a real OpenAI-compatible endpoint (a local Ollama by default), and
// each run reports success, wall time, token usage, cost, and the engine's
// own failure counters (verify failures, retries, stalls, re-splits).
//
// The corpus is intentionally small and versioned in code: a benchmark whose
// goals drift between runs cannot show regressions. Every goal is a Go task
// because Go is the stack divvy can verify offline (go build/go test need no
// network), which keeps runs hermetic and repeatable.
package main

import "strings"

// Goal is one benchmark task. Seed files land in a fresh workspace before the
// run; the prompt is the goal divvy receives; Module is the go module name
// seeded via `go mod init` (empty means the goal runs in a generic workspace).
type Goal struct {
	Name   string
	Module string
	Prompt string
	Seed   map[string]string
}

// Corpus is the fixed goal set. Keep it small: a full bench pass costs real
// model time, and a handful of distinct task shapes (create-from-scratch,
// fix-a-bug, build-a-service) already exposes the engine's weak points:
// decomposition granularity, diagnosis of failing tests, and end-to-end
// acceptance.
var Corpus = []Goal{
	{
		Name:   "string-utils",
		Module: "strutil",
		Prompt: "用 Go 新建包 strutil 并实现三个函数：" +
			"Reverse(s string) string 反转字符串（按 rune，正确处理中文）；" +
			"IsPalindrome(s string) bool 判断回文（忽略大小写，只比较字母和数字）；" +
			"WordCount(s string) int 统计英文单词数。" +
			"为每个函数写表驱动单元测试，覆盖空字符串、大小写混合、含标点的输入。" +
			"只用标准库。",
	},
	{
		Name:   "fix-average",
		Module: "calc",
		Prompt: "workspace 里的 calc 包有 bug：Average 的除数是差一错误，且对空切片会得到 NaN。" +
			"修复它，使 Average 对空切片返回 0、对非空切片返回正确平均值，" +
			"并让 go test ./... 全部通过。不要改动测试文件。",
		Seed: map[string]string{
			"calc.go": `package calc

// Average returns the arithmetic mean of xs.
func Average(xs []int) float64 {
	sum := 0
	for _, x := range xs {
		sum += x
	}
	return float64(sum) / float64(len(xs)+1)
}
`,
			"calc_test.go": `package calc

import "testing"

func TestAverage(t *testing.T) {
	cases := []struct {
		in   []int
		want float64
	}{
		{nil, 0},
		{[]int{}, 0},
		{[]int{1, 2, 3}, 2},
		{[]int{5}, 5},
		{[]int{-2, 2}, 0},
	}
	for _, c := range cases {
		if got := Average(c.in); got != c.want {
			t.Errorf("Average(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
`,
		},
	},
	{
		Name:   "http-health",
		Module: "healthsrv",
		Prompt: "用 Go 写一个 HTTP 服务：监听 :18080，GET /health 返回 200 和字符串 ok，" +
			"其它方法返回 405；并写单元测试。只用标准库。",
	},
}

// goalByName resolves a corpus entry for tests and -goals filtering.
func goalByName(name string) (Goal, bool) {
	for _, g := range Corpus {
		if g.Name == name {
			return g, true
		}
	}
	return Goal{}, false
}

// selectGoals filters the corpus by a comma-separated name list; an empty
// filter selects everything, preserving corpus order.
func selectGoals(filter string) []Goal {
	if strings.TrimSpace(filter) == "" {
		out := make([]Goal, len(Corpus))
		copy(out, Corpus)
		return out
	}
	want := map[string]bool{}
	for _, name := range strings.Split(filter, ",") {
		if n := strings.TrimSpace(name); n != "" {
			want[n] = true
		}
	}
	var out []Goal
	for _, g := range Corpus {
		if want[g.Name] {
			out = append(out, g)
		}
	}
	return out
}
