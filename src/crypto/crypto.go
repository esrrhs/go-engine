package crypto

import "github.com/esrrhs/go-engine/src/crypto/cryptonight"

func Sum(data []byte, algo string, height uint64) []byte {
	return cryptonight.Sum(data, algo, height)
}

func TestSum(algo string) bool {
	return cryptonight.TestSum(algo)
}

func TestAllSum() bool {
	for _, algo := range Algo() {
		if !TestSum(algo) {
			return false
		}
	}
	return true
}

func Algo() []string {
	return cryptonight.Algo()
}
