// +build !amd64

package aes

func CnRoundsGo(dst, src []uint64, rkeys *[40]uint32) {
	CnRoundsGoSoft(dst, src, rkeys)
}

func CnExpandKeyGo(key []uint64, rkeys *[40]uint32) {
	CnExpandKeyGoSoft(dst, src, rkeys)
}
