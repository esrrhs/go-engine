package crypto

import "github.com/esrrhs/go-engine/src/crypto/cryptonight"

func Sum(data []byte, algo string, height uint64) []byte {
	return cryptonight.Sum(data, algo, height)
}

func TestSum(algo string) bool {
	return cryptonight.TestSum(algo)
}
