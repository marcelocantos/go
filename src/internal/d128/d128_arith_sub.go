// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go d128_arith_sub.go.

package d128

// Fast entry: same exponent and a coefficient sum that fits in 112 bits.
// Opposite signs add the magnitudes; same signs subtract them, returning
// directly unless the difference is zero. A zero difference, and every other
// case, tail-calls the out-of-line d128_subtract_slow_rnd_ctxnull.
func d128_subtract_rnd_ctxnull(x, y d128, rounding round, ctxnull *context) d128 {
	xHi49 := d128_hi49(x)
	yHi49 := d128_hi49(y)
	hi49Sum := xHi49 + yHi49
	if ushr_64_64(hi49Sum, 48) == 0 && d128_qExp(x) == d128_qExp(y) {
		if d128_qExpAndSignBit(x) != d128_qExpAndSignBit(y) {
			// Opposite signs, same exponent: this is a magnitude add. Only the sign
			// differs, and x's full UBD word already carries the correct qExp+sign,
			// so add y's coefficient (its top 15 bits are zero) straight onto x's
			// full word and return raw -- no re-extract/rebuild. The guard above
			// (hi49Sum < 2^48) keeps the coefficient carry out of the qExp/sign field.
			sum := add_128_128x128(x.ubdLo64, x.ubdHi64, y.ubdLo64, yHi49)
			return d128{ubdLo64: sum.dw0, ubdHi64: sum.dw1}
		}
		ds := subAbs_d128swap_128x128(x.ubdLo64, xHi49, y.ubdLo64, yHi49)
		if !isZero_bool_128(ds.diffDw0, ds.diffDw1) {
			resultQExpAndSignBit := d128_qExpAndSignBit(x) ^ boolToInt64(ds.swap)
			return d128FromQExpSignBit(resultQExpAndSignBit, ds.diffDw0, ds.diffDw1)
		}
	}
	return d128_subtract_slow_rnd_ctxnull(x, y, rounding, ctxnull)
}

// Slow entry: NaN operands, then everything else the fast entry did not
// resolve, through add's non-finite, zero and finite-nonzero paths with y
// negated (subtract(x, y) = add(x, negate(y))).
//
//go:noinline
func d128_subtract_slow_rnd_ctxnull(x, y d128, rounding round, ctxnull *context) d128 {
	// The go:noinline directive pins this out of line so the fast entry stays the
	// small hot function (see d128_add_slow_rnd_ctxnull).
	if d128_isNAN(x) || d128_isNAN(y) {
		return ctxNanOperand2_ctxnull(ctxnull, x, y)
	}
	yNeg := d128_negate(y)
	if d128_isNonFinite(x) || d128_isNonFinite(yNeg) || d128_isZER(x) || d128_isZER(yNeg) {
		return d128_add_rnd_ctxnull(x, yNeg, rounding, ctxnull)
	}
	return addFnzFnz_rnd_ctxnull(x, yNeg, rounding, ctxnull)
}

func subMagUnalignedFnzFnz_rnd_ctxnull(zSign bool, m, s d128, rounding round, ctxnull *context) d128 {

	mQ := d128_qExp(m)
	mCoeff := d128_u128(m)
	mDigitLen := digitLen_int_128(mCoeff.dw0, mCoeff.dw1)
	sQ := d128_qExp(s)
	sCoeff := d128_u128(s)
	sDigitLen := -1
	gap := mQ - sQ
	if gap < 0 {
		prod := mulPow10_128_128(sCoeff.dw0, sCoeff.dw1, -gap)
		diff := sub_128_128x128(mCoeff.dw0, mCoeff.dw1, prod.dw0, prod.dw1)
		return finalize_128_ctxnull(zSign, mQ, diff, const_RESIDUE_EXACT, rounding, ctxnull)
	}

	headroom := 34 - mDigitLen
	// MQ FAST TAIL (magnitude subtraction): gap fully absorbed into m's headroom
	// (gap <= headroom), so s already sits at the result scale and m*10^gap - s is
	// EXACT — no guard digit, no divide, no residue. diff < 10^34 because |m| > |s|
	// and m*10^gap has <= 34 digits; result exponent sQ is the preferred (minimum)
	// exponent for an exact subtraction.
	if gap <= headroom {
		mScaled := mulPow10_128_128(mCoeff.dw0, mCoeff.dw1, gap)
		diff := sub_128_128x128(mScaled.dw0, mScaled.dw1, sCoeff.dw0, sCoeff.dw1)
		if isLt_pow10_34_bool_128(diff.dw0, diff.dw1) {
			return d128Pack(zSign, sQ, diff.dw0, diff.dw1)
		}
		return finalize_128_ctxnull(zSign, sQ, diff, const_RESIDUE_EXACT, rounding, ctxnull)
	}

	// Fully/Borderline swamped fast path at 34-digit scale.
	shiftM34 := min(gap, headroom)
	qAlign34 := mQ - shiftM34
	shiftS34 := qAlign34 - sQ
	isSwamped := (shiftS34 >= 35)
	if !isSwamped {
		sDigitLen = digitLen_int_128(sCoeff.dw0, sCoeff.dw1)
		isSwamped = (shiftS34 >= sDigitLen)
	}
	if isSwamped {
		var mScaled u128
		if shiftM34 == 0 {
			mScaled = mCoeff
		} else {
			mScaled = mulPow10_128_128(mCoeff.dw0, mCoeff.dw1, shiftM34)
		}
		if cmp_int_128x128_pow10(mScaled.dw0, mScaled.dw1, 33) != 0 { // not 10^33
			raw := const_RESIDUE_LT_HALF
			if !(shiftS34 >= 35 || shiftS34 > sDigitLen) {
				raw = residueFromDecadeWithDigitLen_128(sCoeff.dw0, sCoeff.dw1, sDigitLen)
			}
			z := sub_128_128x128(mScaled.dw0, mScaled.dw1, 1, 0)
			return finalizeSwampedFnz_128_ctxnull(zSign, qAlign34, z, residueSubtractionInverse(raw), rounding, ctxnull)
		}
		// mScaled == 10^33: fall through to the guard path.
	}

	// Guard-digit path (the 10^33 exception and partial overlap).
	headroomWithGuard := 1 + headroom
	shiftMLeft := min(gap, headroomWithGuard)
	qAlign := mQ - shiftMLeft
	shiftSRight := qAlign - sQ

	zCoeff := mulPow10_128_128(mCoeff.dw0, mCoeff.dw1, shiftMLeft)
	if sDigitLen == -1 {
		sDigitLen = digitLen_int_128(sCoeff.dw0, sCoeff.dw1)
	}
	if shiftSRight > sDigitLen {
		borrowed := sub_128_128x128(zCoeff.dw0, zCoeff.dw1, 1, 0)
		return finalizeAddSubFnz_128_ctxnull(zSign, qAlign, borrowed, residueSubtractionInverse(const_RESIDUE_LT_HALF), rounding, ctxnull)
	}
	qr := div_128_pow10_checked_q128res(sCoeff, shiftSRight)
	rawResidue := qr.residue
	zCoeff = sub_128_128x128(zCoeff.dw0, zCoeff.dw1, qr.quotDw0, qr.quotDw1)
	var res residue
	if rawResidue == const_RESIDUE_EXACT {
		res = const_RESIDUE_EXACT
	} else {
		res = residueSubtractionInverse(rawResidue)
		zCoeff = sub_128_128x128(zCoeff.dw0, zCoeff.dw1, 1, 0)
	}
	return finalizeAddSubFnz_128_ctxnull(zSign, qAlign, zCoeff, res, rounding, ctxnull)
}
