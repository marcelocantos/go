// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go div_magic.go.

package d128

// Divides by 10 via a fixed-reciprocal multiply (the barrettStep pattern with a
// hardcoded constant), returning quotient and the residue of the single discarded
// digit.
func div1e1_q128res_128_magic(xDw0, xDw1 int64) quot128Residue {
	magic := u128FromU64(0xCCCC_CCCC_CCCC_CCCD, 0xCCCC_CCCC_CCCC_CCCC)
	shift := 3
	high := mul_256_128x128(xDw0, xDw1, magic.dw0, magic.dw1).hi()
	quot := shr_128_128(high.dw0, high.dw1, shift)
	prod10 := mul_128_128x128(quot.dw0, quot.dw1, 10, 0)
	rem := sub_128_128x128(xDw0, xDw1, prod10.dw0, prod10.dw1).dw0
	res := residueFromDecimalDigit(int(rem))
	return quot128Residue{quot.dw0, quot.dw1, res}
}
