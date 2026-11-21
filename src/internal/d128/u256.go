// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go u256.go.

package d128

import "math/bits"

type u256 struct {
	dw0 int64
	dw1 int64
	dw2 int64
	dw3 int64
}

// lo and hi are the low/high 128-bit halves (primitive-interior, §3.4).
func (x u256) lo() u128 { return u128{dw0: x.dw0, dw1: x.dw1} }

func (x u256) hi() u128 { return u128{dw0: x.dw2, dw1: x.dw3} }

// Primitive-interior sugar (Section 3.4); internal -- core uses bitLen_int_256.
func (x u256) bitLen() int {
	if x.dw3 != 0 {
		return 256 - bits.LeadingZeros64(uint64(x.dw3))
	}
	if x.dw2 != 0 {
		return 192 - bits.LeadingZeros64(uint64(x.dw2))
	}
	if x.dw1 != 0 {
		return 128 - bits.LeadingZeros64(uint64(x.dw1))
	}
	return 64 - bits.LeadingZeros64(uint64(x.dw0))
}

// Primitive-interior sugar (Section 3.4); internal -- core uses digitLen_int_256.
func (x u256) digitLen() int {
	return digitLen_int_256(x.dw0, x.dw1, x.dw2, x.dw3)
}

// u256FromHiLo composes a u256 from its high and low 128-bit halves.
func u256FromHiLo(hi, lo u128) u256 {
	return u256{dw0: lo.dw0, dw1: lo.dw1, dw2: hi.dw0, dw3: hi.dw1}
}

// shr1_256_256 returns `x` shifted right by exactly 1 bit.
// The bottom bit of `hi` flows into the top bit of the new `lo`.
func shr1_256_256(xDw0, xDw1, xDw2, xDw3 int64) u256 {
	lo := fromLoHi_128_64x64(xDw0, xDw1)
	hi := fromLoHi_128_64x64(xDw2, xDw3)
	newLo := orU128(shrU128(lo, 1), shlU128(hi, 127))
	newHi := shrU128(hi, 1)
	return u256FromHiLo(newHi, newLo)
}

// shr_256_256 returns x shifted right by n bits; n in 0..<256.
func shr_256_256(xDw0, xDw1, xDw2, xDw3 int64, n int) u256 {

	lo := fromLoHi_128_64x64(xDw0, xDw1)
	hi := fromLoHi_128_64x64(xDw2, xDw3)
	if n == 0 {
		return u256FromHiLo(hi, lo)
	}
	if n < 128 {
		newLo := orU128(shrU128(lo, n), shlU128(hi, 128-n))
		newHi := shrU128(hi, n)
		return u256FromHiLo(newHi, newLo)
	}
	// 128 <= n < 256
	return u256FromHiLo(u128{dw0: 0, dw1: 0}, shrU128(hi, n-128))
}
