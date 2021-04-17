// +build amd64

package aes

func CnExpandKeyGo(key []uint64, rkeys *[40]uint32) {
	CnExpandKeyAsm(&key[0], rkeys)
}

func CnRoundsGo(dst, src []uint64, rkeys *[40]uint32) {
	CnRoundsAsm(&dst[0], &src[0], &rkeys[0])
}

//go:noescape
func CnExpandKeyAsm(src *uint64, rkey *[40]uint32)

//go:noescape
func CnRoundsAsm(dst, src *uint64, rkeys *uint32)
