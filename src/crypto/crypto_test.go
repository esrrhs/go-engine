package crypto

import (
	"testing"
)

func Test0001(t *testing.T) {
	algos := []string{"cn/0", "cn/1", "cn/2", "cn/r"}
	for _, algo := range algos {
		if !TestSum(algo) {
			t.Error("TestSum fail " + algo)
		}
	}
}
