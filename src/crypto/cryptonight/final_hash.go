package cryptonight

import (
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/esrrhs/go-engine/src/crypto/cryptonight/internal/groestl"
	"github.com/esrrhs/go-engine/src/crypto/cryptonight/internal/jh"
	"github.com/esrrhs/go-engine/src/crypto/cryptonight/internal/skein"
	"hash"
	"sync"
	"unsafe"
)

var hashPool = [...]*sync.Pool{
	{New: func() interface{} { return blake256.New() }},
	{New: func() interface{} { return groestl.New256() }},
	{New: func() interface{} { return jh.New256() }},
	{New: func() interface{} { return skein.New256(nil) }},
}

func (cc *cache) finalHash() []byte {
	hp := hashPool[cc.finalState[0]&0x03]
	h := hp.Get().(hash.Hash)
	h.Reset()
	h.Write((*[200]byte)(unsafe.Pointer(&cc.finalState))[:])
	sum := h.Sum(nil)
	hp.Put(h)

	return sum
}
