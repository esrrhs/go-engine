package crypto

import (
	"fmt"
	"github.com/esrrhs/go-engine/src/crypto/cryptonight"
	"testing"
)

func Test0001(t *testing.T) {
	algos := cryptonight.Algo()
	fmt.Println(algos)
	for _, algo := range algos {
		if !TestSum(algo) {
			t.Error("TestSum fail " + algo)
		}
	}
}
