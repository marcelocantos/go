// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go d128_nextupdown.go.

package d128

// The largest finite magnitude, (10^34 - 1) * 10^6111, with the given sign
// (also finalize's overflow saturation value).
func maxFiniteMagnitude(sign bool) d128 {
	c := sub_128_128x128(pow10_128_dw0(34), pow10_128_dw1(34), 1, 0)
	return decimalFNZ_128(sign, 6111, c.dw0, c.dw1)
}

// The smallest finite (subnormal) magnitude, 1 * 10^-6176, with the given
// sign.
func minFiniteMagnitude(sign bool) d128 {
	return decimalFNZ_64x64(sign, -6176, 1, 0)
}
