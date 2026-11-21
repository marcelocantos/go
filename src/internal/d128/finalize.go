// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go finalize.go.

package d128

func finalize_128_ctxnull(sign bool, qExpIn int, coeffIn u128, residueIn residue, rounding round, ctxnull *context) d128 {
	// Fast path: EXACT residue and already in valid decimal128 range.
	if residueIn == const_RESIDUE_EXACT {
		if isCanonical34_128(qExpIn, coeffIn) {
			return d128Pack(sign, qExpIn, coeffIn.dw0, coeffIn.dw1)
		}
		if isZero_bool_128(coeffIn.dw0, coeffIn.dw1) {
			return d128_zero(sign, qExpIn)
		}
	}
	if isZero_bool_128(coeffIn.dw0, coeffIn.dw1) {
		return finalizeZero_ctxnull(sign, qExpIn, residueIn, rounding, ctxnull)
	}
	return finalize_128_nonTrivial_ctxnull(sign, qExpIn, coeffIn, residueIn, rounding, ctxnull)
}

// Specialized finalize entry point for the unaligned add/subtract paths.
//
// Contract (provable at the call sites, never fires the verifies):
//
//	qExp >= -6176  AND  coeff < 10^35.  coeff may be zero (subtract cancellation).
//
// In the unaligned helpers qAlign = mQ - shift with shift <= qDelta, so
// qAlign >= the smaller operand's exponent >= -6176; and the coefficient is a
// sum/difference of <=34-digit values plus at most one guard digit, so < 10^35
// (=> at most one divide-by-10 of precision trimming).
//
// Because the contract is narrowed and provable, this skips the general
// finalizer's underflow/subnormal diversion and full digit-length scan.
func finalizeAddSubFnz_128_ctxnull(sign bool, qExpIn int, coeffIn u128, residueIn residue, rounding round, ctxnull *context) d128 {

	if isZero_bool_128(coeffIn.dw0, coeffIn.dw1) {
		return finalizeZero_ctxnull(sign, qExpIn, residueIn, rounding, ctxnull)
	}
	coeff0 := coeffIn.dw0
	coeff1 := coeffIn.dw1
	qExp := qExpIn
	res := residueIn
	if cmp_int_128x128_pow10(coeff0, coeff1, 34) >= 0 {
		qr := div_128_pow10_q128res(coeffIn, 1)
		coeff0 = qr.quotDw0
		coeff1 = qr.quotDw1
		res = residueMerge(qr.residue, res)
		qExp += 1
	}
	return roundAndPack_128_ctxnull(sign, qExp, coeff0, coeff1, res, rounding, ctxnull)
}

// Further-specialized entry for the Fully/Borderline Swamped add and subtract
// cases, where the smaller operand lies at or below the result's least-
// significant digit and so contributes only a sub-ULP residue. The caller
// guarantees a narrower contract still:
//
//   - coeff != 0 and coeff < 10^34 (the larger operand filled to <= 34 digits;
//     no precision trim is ever needed)
//   - qExp in [-6176, 6111]
//   - residue != RESIDUE_EXACT (a swamped operand is always inexact)
//
// This collapses finalize to: round, pack, and signal inexact -- skipping the
// 10^34 trim compare and the general round/pack machinery. The lone non-trivial
// case is a round-up that rolls 10^34-1 over the decade at the max exponent.
// (Add passes LT_HALF; subtract passes the borrow-inverted residue. The subtract
// caller pre-screens out coeff == 10^33, whose borrow would need the guard digit.)
func finalizeSwampedFnz_128_ctxnull(sign bool, qExpIn int, coeffIn u128, residueIn residue, rounding round, ctxnull *context) d128 {

	coeff0 := coeffIn.dw0
	coeff1 := coeffIn.dw1
	qExp := qExpIn
	isOdd := (coeff0 & 1) != 0
	if needsRoundUp(rounding, sign, isOdd, residueIn) {
		incremented := add_128_128x128(coeff0, coeff1, 1, 0)
		coeff0 = incremented.dw0
		coeff1 = incremented.dw1
		if cmp_int_128x128_pow10(coeff0, coeff1, 34) == 0 {
			coeff0 = pow10_128_dw0(33)
			coeff1 = pow10_128_dw1(33)
			qExp += 1
			if qExp > 6111 {
				return finalizeOverflow_ctxnull(sign, rounding, ctxnull)
			}
		}
	}
	z := decimalFNZ_64x64(sign, qExp, coeff0, coeff1)
	if ctxnull != nil {
		return ctxSignalInexact_ctxnull(ctxnull, z)
	}
	return z
}

func finalize_128_nonTrivial_ctxnull(sign bool, qExpIn int, coeffIn u128, residueIn residue, rounding round, ctxnull *context) d128 {
	rangeTruncationNeeded := -6176 - qExpIn
	precisionTruncationNeeded := max(digitLen_int_128(coeffIn.dw0, coeffIn.dw1)-34, 0)
	if rangeTruncationNeeded > precisionTruncationNeeded {
		return finalizeUnderflowRegion_128_ctxnull(sign, qExpIn, coeffIn, residueIn, rounding, ctxnull)
	}

	totalResidue := residueIn
	coeff0 := coeffIn.dw0
	coeff1 := coeffIn.dw1
	qExp := qExpIn
	if precisionTruncationNeeded > 0 {
		qr := div_128_pow10_q128res(coeffIn, precisionTruncationNeeded)
		coeff0 = qr.quotDw0
		coeff1 = qr.quotDw1
		totalResidue = residueMerge(qr.residue, totalResidue)
		qExp += precisionTruncationNeeded

	}

	return roundAndPack_128_ctxnull(sign, qExp, coeff0, coeff1, totalResidue, rounding, ctxnull)
}

func finalizeZero_ctxnull(sign bool, qExp int, res residue, rounding round, ctxnull *context) d128 {

	var z d128
	isOdd := false
	if res != const_RESIDUE_EXACT && needsRoundUp(rounding, sign, isOdd, res) {
		if qExp < -6176 {
			return finalizeUnderflowRegion_128_ctxnull(sign, qExp, u128{1, 0}, res, rounding, ctxnull)
		}
		z = decimalFNZ_64x64(sign, qExp, 1, 0)
	} else {
		z = d128_zero(sign, qExp)
	}
	if res != const_RESIDUE_EXACT && ctxnull != nil {
		return ctxSignalInexact_ctxnull(ctxnull, z)
	}
	return z
}

// Overflow saturation, shared with the arithmetic cores: the infinity of the
// given sign, or the largest finite magnitude when the rounding direction does
// not overflow to infinity. Signals overflow and inexact when ctxnull is
// non-nil.
func finalizeOverflow_ctxnull(sign bool, rounding round, ctxnull *context) d128 {
	var z d128
	if roundingOverflowsToInfinity(rounding, sign) {
		z = d128_infinity(sign)
	} else {
		z = maxFiniteMagnitude(sign)
	}
	if ctxnull != nil {
		return ctxSignalInexactOverflow_ctxnull(ctxnull, z)
	}
	return z
}

func finalizeUnderflowRegion_128_ctxnull(sign bool, qExp int, coeff u128, res residue, rounding round, ctxnull *context) d128 {

	truncationNeeded := -6176 - qExp
	digitLen := digitLen_int_128(coeff.dw0, coeff.dw1)
	if truncationNeeded > digitLen {
		return finalizeUnderflowFullySwamped(sign, rounding, ctxnull)
	}
	if truncationNeeded == digitLen {
		scaleResidue := residueFromDecadeWithDigitLen_128(coeff.dw0, coeff.dw1, digitLen)
		totalResidue := residueMerge(scaleResidue, res)
		return finalizeUnderflowExactBoundary_ctxnull(sign, totalResidue, rounding, ctxnull)
	}

	qr := div_128_pow10_q128res(coeff, truncationNeeded)
	totalResidue := residueMerge(qr.residue, res)
	return finalizeSubnormal_ctxnull(sign, qr.quotDw0, qr.quotDw1, totalResidue, rounding, ctxnull)
}

func finalizeUnderflowRegion_256_ctxnull(sign bool, qExp int, u256In u256, res residue, rounding round, ctxnull *context) d128 {

	truncationNeeded := -6176 - qExp
	digitLen := digitLen_int_256(u256In.dw0, u256In.dw1, u256In.dw2, u256In.dw3)
	if truncationNeeded > digitLen {
		return finalizeUnderflowFullySwamped(sign, rounding, ctxnull)
	}
	if truncationNeeded == digitLen {
		scaleResidue := residueFromDecadeWithDigitLen_256(u256In, digitLen)
		totalResidue := residueMerge(scaleResidue, res)
		return finalizeUnderflowExactBoundary_ctxnull(sign, totalResidue, rounding, ctxnull)
	}

	qr := div_256_pow10_q128res(u256In, truncationNeeded)
	totalResidue := residueMerge(qr.residue, res)
	return finalizeSubnormal_ctxnull(sign, qr.quotDw0, qr.quotDw1, totalResidue, rounding, ctxnull)
}

func finalizeUnderflowFullySwamped(sign bool, rounding round, ctxnull *context) d128 {
	var z d128
	if roundingUnderflowsToZero(rounding, sign) {
		z = d128_zero(sign, -6176)
	} else {
		z = minFiniteMagnitude(sign)
	}
	if ctxnull != nil {
		return ctxSignalInexactUnderflow_ctxnull(ctxnull, z)
	}
	return z
}

func finalizeUnderflowExactBoundary_ctxnull(sign bool, res residue, rounding round, ctxnull *context) d128 {
	var z d128
	isOdd := false
	if needsRoundUp(rounding, sign, isOdd, res) {
		z = minFiniteMagnitude(sign)
	} else {
		z = d128_zero(sign, -6176)
	}
	if ctxnull != nil {
		return ctxSignalInexactUnderflow_ctxnull(ctxnull, z)
	}
	return z
}

func finalizeSubnormal_ctxnull(sign bool, coeffDw0, coeffDw1 int64, res residue, rounding round, ctxnull *context) d128 {

	coeffT0 := coeffDw0
	coeffT1 := coeffDw1
	qExpT := -6176
	isOdd := (coeffT0 & 1) != 0
	if res != const_RESIDUE_EXACT && needsRoundUp(rounding, sign, isOdd, res) {
		rounded := add_128_128x128(coeffT0, coeffT1, 1, 0)
		coeffT0 = rounded.dw0
		coeffT1 = rounded.dw1

		if cmp_int_128x128_pow10(coeffT0, coeffT1, 34) == 0 {
			coeffT0 = pow10_128_dw0(33)
			coeffT1 = pow10_128_dw1(33)
			qExpT += 1
		}
	}
	z := decimalFNZ_64x64(sign, qExpT, coeffT0, coeffT1)
	if res != const_RESIDUE_EXACT && ctxnull != nil {
		return ctxSignalInexactUnderflow_ctxnull(ctxnull, z)
	}
	return z
}

func roundAndPack_128_ctxnull(sign bool, qExpIn int, coeffDw0, coeffDw1 int64, totalResidue residue, rounding round, ctxnull *context) d128 {
	qExp := qExpIn
	coeff0 := coeffDw0
	coeff1 := coeffDw1

	applyRounding := totalResidue != const_RESIDUE_EXACT
	isOdd := (coeff0 & 1) != 0
	if applyRounding && needsRoundUp(rounding, sign, isOdd, totalResidue) {
		incremented := add_128_128x128(coeff0, coeff1, 1, 0)
		coeff0 = incremented.dw0
		coeff1 = incremented.dw1
		if cmp_int_128x128_pow10(coeff0, coeff1, 34) == 0 {
			coeff0 = pow10_128_dw0(33)
			coeff1 = pow10_128_dw1(33)
			qExp += 1
		}
	}

	if qExp > 6111 {
		qExcess := qExp - 6111
		if digitLen_int_128(coeff0, coeff1)+qExcess <= 34 {
			return finalizeClamping_128(sign, coeff0, coeff1, qExp)
		}
		return finalizeOverflow_ctxnull(sign, rounding, ctxnull)
	}
	z := decimalFNZ_64x64(sign, qExp, coeff0, coeff1)
	if applyRounding && ctxnull != nil {
		return ctxSignalInexact_ctxnull(ctxnull, z)
	}
	return z
}

// Whether (qExp, coeff) is already a canonical decimal128: coeff < 10^34 and
// qExp in range.
func isCanonical34_128(qExp int, coeff u128) bool {
	return isLt_pow10_34_bool_128(coeff.dw0, coeff.dw1) && qExp >= -6176 && qExp <= 6111
}

func finalizeClamping_128(sign bool, coeffDw0, coeffDw1 int64, qExp int) d128 {
	qExcess := qExp - 6111

	coeffScaled := mulPow10_128_128(coeffDw0, coeffDw1, qExcess)

	return decimalFNZ_64x64(sign, 6111, coeffScaled.dw0, coeffScaled.dw1)
}

// finalize_256_ctxnull finalizes a 256-bit wide product (multiply, fma): the 128-bit finalizer when
// the high half is zero, else the full-width path.
func finalize_256_ctxnull(sign bool, qExpIn int, u256In u256, residueIn residue, rounding round, ctxnull *context) d128 {
	if isZero_bool_128(u256In.dw2, u256In.dw3) {
		return finalize_128_ctxnull(sign, qExpIn, u256In.lo(), residueIn, rounding, ctxnull)
	}
	return finalizeFullWidth_256_ctxnull(sign, qExpIn, u256In, residueIn, rounding, ctxnull)
}

func finalizeFullWidth_256_ctxnull(sign bool, qExpIn int, u256In u256, residueIn residue, rounding round, ctxnull *context) d128 {

	rangeTruncationNeeded := -6176 - qExpIn
	precisionTruncationNeeded := max(digitLen_int_256(u256In.dw0, u256In.dw1, u256In.dw2, u256In.dw3)-34, 0)
	if rangeTruncationNeeded > precisionTruncationNeeded {
		return finalizeUnderflowRegion_256_ctxnull(sign, qExpIn, u256In, residueIn, rounding, ctxnull)
	}

	qr := div_256_pow10_q128res(u256In, precisionTruncationNeeded)
	totalResidue := residueMerge(qr.residue, residueIn)
	qExp := qExpIn + precisionTruncationNeeded

	return roundAndPack_128_ctxnull(sign, qExp, qr.quotDw0, qr.quotDw1, totalResidue, rounding, ctxnull)
}
