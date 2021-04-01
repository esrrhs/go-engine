package cryptonight

import (
	"testing"
)

func Test0001(t *testing.T) {
	if !TestSum(0) {
		t.Error("TestSum fail 0")
	}
	if !TestSum(1) {
		t.Error("TestSum fail 1")
	}
	if !TestSum(2) {
		t.Error("TestSum fail 2")
	}
}

// Here we don't make a seperate template function, as we want the function address
// to be known at link time so the result can be more accurate.
func BenchmarkSum(b *testing.B) {
	b.Run("v0", func(b *testing.B) {
		b.N = 100
		for i := 0; i < b.N; i++ {
			Sum(benchData[i&0x03], 0)
		}
	})
	b.Run("v1", func(b *testing.B) {
		b.N = 100
		for i := 0; i < b.N; i++ {
			Sum(benchData[i&0x03], 1)
		}
	})
	b.Run("v2", func(b *testing.B) {
		b.N = 100
		for i := 0; i < b.N; i++ {
			Sum(benchData[i&0x03], 2)
		}
	})

	b.Run("v0-parallel", func(b *testing.B) {
		b.N = 100
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				Sum(benchData[i&0x03], 0)
				i++
			}
		})
	})
	b.Run("v1-parallel", func(b *testing.B) {
		b.N = 100
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				Sum(benchData[i&0x03], 1)
				i++
			}
		})
	})
	b.Run("v2-parallel", func(b *testing.B) {
		b.N = 100
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				Sum(benchData[i&0x03], 2)
				i++
			}
		})
	})
}
