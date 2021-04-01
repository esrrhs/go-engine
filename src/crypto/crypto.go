package crypto

import "github.com/esrrhs/go-engine/src/crypto/cryptonight"

func Sum(data []byte, algo string) []byte {
	switch algo {
	case "cn/0":
		return cryptonight.Sum(data, 0)
	case "cn/1":
		return cryptonight.Sum(data, 1)
	case "cn/2":
		return cryptonight.Sum(data, 2)
	}
	return nil
}

func TestSum(algo string) bool {
	switch algo {
	case "cn/0":
		return cryptonight.TestSum(0)
	case "cn/1":
		return cryptonight.TestSum(1)
	case "cn/2":
		return cryptonight.TestSum(2)
	}
	return false
}
