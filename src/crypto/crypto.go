package crypto

import "github.com/esrrhs/go-engine/src/crypto/cryptonight"

func Sum(data []byte, algo string, height uint64) []byte {
	switch algo {
	case "cn/0":
		return cryptonight.Sum(data, 0, height)
	case "cn/1":
		return cryptonight.Sum(data, 1, height)
	case "cn/2":
		return cryptonight.Sum(data, 2, height)
	case "cn/r":
		return cryptonight.Sum(data, 4, height)
	case "cn/fast":
		return cryptonight.Sum(data, 5, height)
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
	case "cn/r":
		return cryptonight.TestSum(4)
	case "cn/fast":
		return cryptonight.TestSum(5)
	}
	return false
}
