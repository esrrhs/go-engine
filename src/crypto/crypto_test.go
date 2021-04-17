package crypto

import (
	"fmt"
	"github.com/esrrhs/go-engine/src/crypto/cryptonight/inter/aes"
	"testing"
)

func Test0001(t *testing.T) {
	fmt.Println(Algo())
	if !TestAllSum() {
		t.Error("TestSum fail")
	}
	aes.UseSoft(true)
	if !TestAllSum() {
		t.Error("TestSum fail")
	}
}
