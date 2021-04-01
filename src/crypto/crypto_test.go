package crypto

import (
	"testing"
)

func Test0001(t *testing.T) {
	algos := []string{"cn/0"}
	for _, algo := range algos {
		if !TestSum(algo) {
			t.Error("TestSum fail " + algo)
		}
	}
}
