// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go mul_pow10.go.

package d128

// mulPow10_128_128 returns the low 128 bits of x * 10^pow10 (wrapping).
func mulPow10_128_128(xDw0, xDw1 int64, pow10 int) u128 {
	return mul_128_128x128(xDw0, xDw1, pow10_128_dw0(pow10), pow10_128_dw1(pow10))
}
