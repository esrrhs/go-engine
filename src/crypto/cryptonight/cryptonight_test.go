package cryptonight

import (
	"testing"
)

func Test0000(t *testing.T) {
	if !TestSum(0) {
		t.Error("TestSum fail 0")
	}
}

func Test0001(t *testing.T) {
	if !TestSum(1) {
		t.Error("TestSum fail 1")
	}
}

func Test0002(t *testing.T) {
	if !TestSum(2) {
		t.Error("TestSum fail 2")
	}
}

func Test0004(t *testing.T) {
	if !TestSum(4) {
		t.Error("TestSum fail 4")
	}
}

func Test0005(t *testing.T) {
	if !TestSum(5) {
		t.Error("TestSum fail 5")
	}
}

func Test0006(t *testing.T) {
	if !TestSum(6) {
		t.Error("TestSum fail 6")
	}
}

func Test0007(t *testing.T) {
	if !TestSum(7) {
		t.Error("TestSum fail 7")
	}
}

func Test0008(t *testing.T) {
	if !TestSum(8) {
		t.Error("TestSum fail 7")
	}
}
