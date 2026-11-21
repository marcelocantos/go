// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go u256_mul.go.

package d128

import "math/bits"

// mul_256_128x128 returns the full-width 128*128->256 product.
func mul_256_128x128(xDw0, xDw1, yDw0, yDw1 int64) u256 {
	a0, a1 := uint64(xDw0), uint64(xDw1)
	b0, b1 := uint64(yDw0), uint64(yDw1)
	h00, l00 := bits.Mul64(a0, b0)
	h01, l01 := bits.Mul64(a0, b1)
	h10, l10 := bits.Mul64(a1, b0)
	h11, l11 := bits.Mul64(a1, b1)
	r0 := l00
	r1, c1 := bits.Add64(h00, l01, 0)
	r1, c2 := bits.Add64(r1, l10, 0)
	r2, c3 := bits.Add64(h01, h10, 0)
	r2, c4 := bits.Add64(r2, l11, 0)
	r2, c5 := bits.Add64(r2, c1+c2, 0)
	r3 := h11 + c3 + c4 + c5
	return u256{int64(r0), int64(r1), int64(r2), int64(r3)}
}
