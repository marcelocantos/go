// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go d128_arith_add.go.

package d128

// Fast entry: same sign and exponent, and a coefficient sum that fits in 112
// bits, returns directly (no rounding, no flags). Every other case tail-calls
// the out-of-line d128_add_slow_rnd_ctxnull.
func d128_add_rnd_ctxnull(x, y d128, rounding round, ctxnull *context) d128 {
	xHi49 := d128_hi49(x)
	yHi49 := d128_hi49(y)
	hi49Sum := xHi49 + yHi49
	if ushr_64_64(hi49Sum, 48) == 0 && d128_qExpAndSignBit(x) == d128_qExpAndSignBit(y) {
		// Same sign and exponent; sum fits in 112 bits (+ carry). No rounding.
		// Add y's coefficient (its top 15 bits are zero) straight onto x's full
		// UBD word: qExpAndSign(x) rides along untouched, so there is no
		// re-extract/rebuild. The guard above (hi49Sum < 2^48) guarantees the
		// coefficient carry cannot reach the qExp/sign field.
		sum := add_128_128x128(x.ubdLo64, x.ubdHi64, y.ubdLo64, yHi49)
		return d128{ubdLo64: sum.dw0, ubdHi64: sum.dw1}
	}
	return d128_add_slow_rnd_ctxnull(x, y, rounding, ctxnull)
}

// Slow entry: the non-finite, zero, and general finite-nonzero cases. Kept out
// of line so the fast entry stays small.
//
//go:noinline
func d128_add_slow_rnd_ctxnull(x, y d128, rounding round, ctxnull *context) d128 {
	// Kept out of line so the fast entry stays a small, i-cache-friendly hot function.
	// Go has no always_inline, and the fast entry's guard + inline arithmetic already
	// exceeds the ~80 inline budget, so it does not collapse into Add (unlike the
	// native ports' operators); the split's Go payoff is hot/cold code separation.
	// The go:noinline directive pins that contract.
	// Aligned unlike-sign magnitude subtraction, hoisted ahead of the cold
	// ladder. hi49Sum fitting in 48 bits proves BOTH operands finite (nonfinite
	// coefficients are >= 10^34, whose bit 48 is set), and the sole caller is
	// the fast entry, which consumed every aligned like-sign pair with this
	// property — so the signs differ here. Both coefficients are canonical and
	// unscaled, so the difference is < 10^34 and packs raw; swapped means
	// |x| < |y|, and the result takes the larger magnitude's sign. Mirror of
	// the subtract fast path's subAbs arm.
	xHi49 := d128_hi49(x)
	yHi49 := d128_hi49(y)
	hi49Sum := xHi49 + yHi49
	if ushr_64_64(hi49Sum, 48) == 0 && d128_qExp(x) == d128_qExp(y) {

		ds := subAbs_d128swap_128x128(x.ubdLo64, xHi49, y.ubdLo64, yHi49)
		if isZero_bool_128(ds.diffDw0, ds.diffDw1) {
			return d128_zero(rounding == const_ROUND_TOWARD_NEGATIVE, d128_qExp(x))
		}
		resultQExpAndSignBit := d128_qExpAndSignBit(x) ^ boolToInt64(ds.swap)
		return d128FromQExpSignBit(resultQExpAndSignBit, ds.diffDw0, ds.diffDw1)
	}
	if d128_isNonFinite(x) || d128_isNonFinite(y) {
		return addNonFinite_rnd_ctxnull(x, y, ctxnull)
	}
	if d128_isZER(x) {
		if d128_isZER(y) {
			resultSign := (d128_signFlag(x) && d128_signFlag(y)) ||
				(d128_signFlag(x) != d128_signFlag(y) && rounding == const_ROUND_TOWARD_NEGATIVE)
			return d128_zero(resultSign, min(d128_qExp(x), d128_qExp(y)))
		}
		return preferMinimumQExp_exact(y, d128_qExp(x))
	}
	if d128_isZER(y) {
		return preferMinimumQExp_exact(x, d128_qExp(y))
	}
	return addFnzFnz_rnd_ctxnull(x, y, rounding, ctxnull)
}

// Finite-nonzero + finite-nonzero add core: nearly aligned operands are
// aligned exactly, the rest go to the unaligned kernels. Subtract delegates
// here through subtract(x, y) = add(x, negate(y)), so it is module-internal
// rather than file-private.
func addFnzFnz_rnd_ctxnull(x, y d128, rounding round, ctxnull *context) d128 {
	qAligned := min(d128_qExp(x), d128_qExp(y))
	absXShift := d128_qExp(x) - qAligned // one of these is zero
	absYShift := d128_qExp(y) - qAligned
	if absXShift+absYShift <= 4 {
		// Nearly aligned: align to qAligned without overflowing 128 bits.
		var xCoeffAligned u128
		if absXShift == 0 {
			xCoeffAligned = d128_u128(x)
		} else {
			xCoeffAligned = mulPow10_128_128(x.ubdLo64, d128_hi49(x), absXShift)
		}
		var yCoeffAligned u128
		if absYShift == 0 {
			yCoeffAligned = d128_u128(y)
		} else {
			yCoeffAligned = mulPow10_128_128(y.ubdLo64, d128_hi49(y), absYShift)
		}

		var resultSign bool
		var resultCoeff u128
		if d128_signFlag(x) == d128_signFlag(y) {
			resultSign = d128_signFlag(x)
			resultCoeff = add_128_128x128(xCoeffAligned.dw0, xCoeffAligned.dw1, yCoeffAligned.dw0, yCoeffAligned.dw1)
		} else {
			ds := subAbs_d128swap_128x128(xCoeffAligned.dw0, xCoeffAligned.dw1, yCoeffAligned.dw0, yCoeffAligned.dw1)
			if isZero_bool_128(ds.diffDw0, ds.diffDw1) {
				return d128_zero(rounding == const_ROUND_TOWARD_NEGATIVE, qAligned)
			}
			resultSign = d128_signFlag(x) != ds.swap
			resultCoeff = u128{ds.diffDw0, ds.diffDw1}
		}
		if isLt_pow10_34_bool_128(resultCoeff.dw0, resultCoeff.dw1) {
			return d128Pack(resultSign, qAligned, resultCoeff.dw0, resultCoeff.dw1)
		}
		return finalize_128_ctxnull(resultSign, qAligned, resultCoeff, const_RESIDUE_EXACT, rounding, ctxnull)
	}
	if d128_signFlag(x) == d128_signFlag(y) {
		return addFnzFnzUnalignedLikeSign_rnd_ctxnull(x, y, rounding, ctxnull)
	}
	return addFnzFnzUnalignedUnlikeSign_rnd_ctxnull(x, y, rounding, ctxnull)
}

func addFnzFnzUnalignedLikeSign_rnd_ctxnull(x, y d128, rounding round, ctxnull *context) d128 {
	xQ := d128_qExp(x)
	yQ := d128_qExp(y)

	var m, n d128
	var mQ, nQ int
	if xQ > yQ {
		m, n = x, y
		mQ = xQ
		nQ = yQ
	} else {
		m, n = y, x
		mQ = yQ
		nQ = xQ
	}
	mDigitLen := d128_digitLen(m)
	qDelta := mQ - nQ

	headroom := 34 - mDigitLen
	shiftLeft := min(max(headroom, 0), qDelta)
	var zCoeff u128
	if shiftLeft == 0 {
		zCoeff = d128_u128(m)
	} else {
		zCoeff = mulPow10_128_128(m.ubdLo64, d128_hi49(m), shiftLeft)
	}
	qAlign := mQ - shiftLeft
	shiftRight := qAlign - nQ
	// MQ FAST TAIL: gap fully absorbed into m's headroom (shiftRight == 0), so n
	// already sits at the result scale and, being the same sign, its coefficient
	// simply adds. The sum is EXACT: skip the div-by-zero and residue finalize.
	if shiftRight == 0 {
		nCoeff := d128_u128(n)
		sum := add_128_128x128(zCoeff.dw0, zCoeff.dw1, nCoeff.dw0, nCoeff.dw1)
		if isLt_pow10_34_bool_128(sum.dw0, sum.dw1) {
			return d128Pack(d128_signFlag(m), qAlign, sum.dw0, sum.dw1)
		}
		return finalize_128_ctxnull(d128_signFlag(m), qAlign, sum, const_RESIDUE_EXACT, rounding, ctxnull)
	}
	// Swamped fast path: n lies strictly below the result's LSD (LT_HALF residue).
	if shiftRight >= 35 || shiftRight > d128_digitLen(n) {
		return finalizeSwampedFnz_128_ctxnull(d128_signFlag(m), qAlign, zCoeff, const_RESIDUE_LT_HALF, rounding, ctxnull)
	}
	qr := div_128_pow10_checked_q128res(d128_u128(n), shiftRight)
	res := qr.residue
	zCoeff = add_128_128x128(zCoeff.dw0, zCoeff.dw1, qr.quotDw0, qr.quotDw1)
	return finalizeAddSubFnz_128_ctxnull(d128_signFlag(m), qAlign, zCoeff, res, rounding, ctxnull)
}

func addFnzFnzUnalignedUnlikeSign_rnd_ctxnull(x, y d128, rounding round, ctxnull *context) d128 {

	cmp := cmpMagFnzFnz(x, y)
	if cmp > 0 {
		return subMagUnalignedFnzFnz_rnd_ctxnull(d128_signFlag(x), x, y, rounding, ctxnull)
	}
	if cmp < 0 {
		return subMagUnalignedFnzFnz_rnd_ctxnull(d128_signFlag(y), y, x, rounding, ctxnull)
	}
	return d128_zero(rounding == const_ROUND_TOWARD_NEGATIVE, min(d128_qExp(x), d128_qExp(y)))
}

func addNonFinite_rnd_ctxnull(x, y d128, ctxnull *context) d128 {
	if d128_isNAN(x) || d128_isNAN(y) {
		return ctxNanOperand2_ctxnull(ctxnull, x, y)
	}

	if d128_isFIN(x) {
		return y
	}
	if d128_isFIN(y) {
		return x
	}
	if d128_signFlag(x) == d128_signFlag(y) {
		return x
	}
	return ctxSignalInvalidOperation_ctxnull(ctxnull, causeMagnitudeSubtractionOfInfinities, const_D128_QNAN0)
}

// preferMinimumQExp_exact lowers x's exponent toward otherExp by scaling its coefficient up into the
// free digits, exactly (the value is unchanged). Returns x unchanged when
// there is no headroom or otherExp >= qExp(x). Shared with fma.
func preferMinimumQExp_exact(x d128, otherExp int) d128 {
	xSign := d128_signFlag(x)
	xQ := d128_qExp(x)
	xCoeff := d128_u128(x)
	delta := xQ - otherExp
	if delta <= 0 {
		return x
	}
	headroom := max(0, 34-d128_digitLen(x))
	if headroom == 0 {
		return x
	}
	expRoom := xQ + 6176
	shiftLeft := min(min(headroom, delta), expRoom)
	var zCoeff u128
	if shiftLeft == 0 {
		zCoeff = xCoeff
	} else {
		zCoeff = mulPow10_128_128(xCoeff.dw0, xCoeff.dw1, shiftLeft)
	}
	zQ := xQ - shiftLeft

	return d128Pack(xSign, zQ, zCoeff.dw0, zCoeff.dw1)
}
