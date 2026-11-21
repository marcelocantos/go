// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go d128_arith_compare.go.

package d128

// Magnitude compare of two finite-nonzero values: -1 / 0 / +1. Shared with the
// add and subtract cores.
func cmpMagFnzFnz(x, y d128) int {

	qDelta := d128_qExp(x) - d128_qExp(y)
	if qDelta >= 34 {
		return 1
	}
	if qDelta <= -34 {
		return -1
	}
	xCoeff := d128_u128(x)
	yCoeff := d128_u128(y)
	xBitLen := bitLen_int_128(xCoeff.dw0, xCoeff.dw1)
	yBitLen := bitLen_int_128(yCoeff.dw0, yCoeff.dw1)
	xDigitLen := digitLen_int_128_withBitLen(xCoeff.dw0, xCoeff.dw1, xBitLen)
	yDigitLen := digitLen_int_128_withBitLen(yCoeff.dw0, yCoeff.dw1, yBitLen)
	cmpBExp := compareBExpMagnitudeFnzFnz(d128_qExp(x), xBitLen, xDigitLen, d128_qExp(y), yBitLen, yDigitLen)
	if cmpBExp != 0 {
		return cmpBExp
	}
	xAligned := xCoeff
	yAligned := yCoeff
	// |qDelta| is bounded by digitLen_max (34 for decimal128) because sciExps
	// match, so qDelta = digitLen_y - digitLen_x. The binary-bracket pre-check
	// makes typical qDelta small but doesn't tighten this worst-case bound.
	if qDelta > 0 {
		xAligned = mulPow10_128_128(xAligned.dw0, xAligned.dw1, qDelta)
	} else if qDelta < 0 {
		yAligned = mulPow10_128_128(yAligned.dw0, yAligned.dw1, -qDelta)
	}
	return cmp_int_128x128(xAligned.dw0, xAligned.dw1, yAligned.dw0, yAligned.dw1)
}

func compareBExpMagnitudeFnzFnz(xQExp, xBitLen, xDigitLen, yQExp, yBitLen, yDigitLen int) int {
	xE := eExpFnz(xQExp, xDigitLen)
	yE := eExpFnz(yQExp, yDigitLen)
	if xE != yE {
		return ((xE - yE) >> 63) | 1 // -1 or 1
	}
	if calcBExpMin(xBitLen, xQExp) > calcBExpMax(yBitLen, yQExp) {
		return 1
	}
	if calcBExpMax(xBitLen, xQExp) < calcBExpMin(yBitLen, yQExp) {
		return -1
	}
	return 0
}

func eExpFnz(qExp, digitLen int) int {

	return qExp + digitLen - 1
}
