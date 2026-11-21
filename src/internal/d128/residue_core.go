// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go residue_core.go.

package d128

// residueMerge merges two residues from a single quantity (left = the higher-order
// truncation).
func residueMerge(leftResidue, rightResidue residue) residue {

	return (leftResidue | rightResidue>>1 | rightResidue&1) & 3
}

// The residue of the additive inverse (subtraction) of a value.
func residueSubtractionInverse(res residue) residue {

	// EXACT->EXACT, LT_HALF->GT_HALF, HALF->HALF, GT_HALF->LT_HALF, packed nybbles.
	const const_INVERSE_MAP = 0x1230
	return (const_INVERSE_MAP >> uint(res<<2)) & 0x3
}

// Residue of truncating an entire decade (all digitLen digits) from a value.
func residueFromDecadeWithDigitLen_128(dw0, dw1 int64, digitLen int) residue {
	// Not mergeable with residueFromRemainderDivisor_128: digitLen could be 39
	// but 10^39 does not fit in 128 bits.
	if digitLen >= 0 && digitLen <= 38 {
		halfPow10 := shr_128_128(pow10_128_dw0(digitLen), pow10_128_dw1(digitLen), 1)
		c := cmp_int_128x128(dw0, dw1, halfPow10.dw0, halfPow10.dw1)
		ge := 0
		if c >= 0 {
			ge = 1
		}
		eq := 0
		if c == 0 {
			eq = 1
		}
		return (ge << 1) | (1 ^ eq)
	}

	return const_RESIDUE_LT_HALF
}

func residueFromDecadeWithDigitLen_256(v u256, digitLen int) residue {
	if isZero_bool_128(v.dw2, v.dw3) {

		return residueFromDecadeWithDigitLen_128(v.dw0, v.dw1, digitLen)
	}
	if digitLen >= 39 && digitLen <= 77 {
		i := digitLen - 39
		halfPow10 := shr1_256_256(pow10_256_dw0(i), pow10_256_dw1(i), pow10_256_dw2(i), pow10_256_dw3(i))
		ge := int(boolToInt64(!v.less(halfPow10)))
		eq := int(boolToInt64(v == halfPow10))
		return (ge << 1) | (1 ^ eq)
	}
	impossible("invalid digitLen")
	return 0 // unreachable
}
