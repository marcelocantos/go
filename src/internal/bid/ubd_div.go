// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// This package's divide on UBD operands. ubdDivTTE128, ubdDivWide128 and
// ubdDivLong128 are div128 (for ties to even), divWide128 and divLong128
// (their comments explain the method) with the BID decoding (divPair128,
// unpackFirst128, unpackRest128) replaced by UBD's, the specials decided on
// UBD, and the packing and rounding by ubdPackWord, ubdRoundFace128 and
// ubdRound128; the arithmetic is unchanged. Nothing converts.

// ubdDivPair128 is divPair128 on UBD: the coefficients' high words, the
// exponent difference (the same biased or not), the quotient's sign, and
// whether both operands are finite.
func ubdDivPair128(x, y UBD128) (xhi, yhi uint64, exp int, neg, ok bool) {
	ok = x.Hi>>60 != UBDSpecialTag && y.Hi>>60 != UBDSpecialTag
	exp = int(int64(x.Hi)>>UBDExpShift) - int(int64(y.Hi)>>UBDExpShift)
	return x.Hi & UBDCoeffMaskHi, y.Hi & UBDCoeffMaskHi, exp, (x.Hi^y.Hi)&UBDSignHi != 0, ok
}

// ubdDivTTE128, the divide entry the compiler calls, is div128 on UBD for
// ties to even, written into the entry so that a divide is one call with
// one frame, where it was the entry and then a ubdDiv128 (-6 to -8% on
// word operands in BenchmarkUBDKernel; money 12.59 ns against 11.86, BID's
// div128 15.58). ubdDivRnd128,
// which no compiled code calls, calls it for ToNearestEven and
// ubdDivLong128, which takes every mode, otherwise.
func ubdDivTTE128(x, y UBD128) UBD128 {
	const mode = ToNearestEven
	xhi, yhi, exp, neg, ok := ubdDivPair128(x, y)
	a, b := x.Lo, y.Lo
	if !ok || mode > ToNearestAway {
		return ubdDivLong128(x, y, mode)
	}
	if yhi != 0 {
		return ubdDivWide128(x, y, mode)
	}
	if a|xhi == 0 || b == 0 {
		// A zero operand has its result without a division, and these
		// are the two that data holds: a zero amount divided, which is
		// a zero at the preferred exponent, and a division by zero,
		// which is an infinity. Both have the sign of the quotient.
		// The rest are divLong128's: 0 / 0, an exponent outside the
		// format, and a dividend that only reads as zero, because its
		// coefficient is not canonical.
		if b != 0 {
			if r, ok := ubdPackWord(neg, 0, 0, exp); ok {
				return r
			}
		} else if a|xhi != 0 && !u128GtMaxCoeff(Uint128{Hi: xhi, Lo: a}) {
			return ubdInf(neg)
		}
		return ubdDivLong128(x, y, mode)
	}
	db := wordDigits(b)
	if b == pow10word[(db-1)&31] {
		// A power of ten divides by moving the point.
		q, k := Uint128{Hi: xhi, Lo: a}, db-1
		if u128GtMaxCoeff(q) {
			return ubdDivLong128(x, y, mode)
		}
		if k > 0 {
			q, k = strip128(q, k, 1)
		}
		if r, ok := ubdPackWord(neg, q.Hi, q.Lo, exp-k); ok {
			return r
		}
		return ubdDivLong128(x, y, mode)
	}

	// An exact quotient of one-word operands, the usual result of dividing
	// an amount by a count or a small rate, is found without the long
	// division. b divides a × 10^j for some j exactly when what b has
	// besides twos and fives, g, divides a; the least such j is then the
	// twos or the fives of b in excess of those of a, whichever is more,
	// and the quotient a × 10^j / b is the result at j places below the
	// preferred exponent, which is where the long division's quotient
	// would be stripped to. A division that is not exact leaves here at
	// the test of g, after a shift, a multiply and one division
	// instruction beside the work below.
	if xhi == 0 {
		tb := bits.TrailingZeros64(b)
		fb, g := fives(b >> (uint(tb) & 63))
		if a%g == 0 {
			fa, _ := fives(a)
			j := max(tb-bits.TrailingZeros64(a), fb-fa, 0)
			if j < len(pow10tab64) {
				if hi, lo := bits.Mul64(a, pow10word[j&31]); hi == 0 {
					if r, ok := ubdPackWord(neg, 0, lo/b, exp-j); ok {
						return r
					}
					return ubdDivLong128(x, y, mode)
				}
			}
		}
	}

	// The reciprocal, when it is needed, is asked for first: it takes the
	// divisor alone, it is the longest chain here, and nothing else is
	// yet in a register for the call to displace.
	var s uint
	var d, v uint64
	small := b>>32 == 0
	if !div64Native && !small {
		s = uint(bits.LeadingZeros64(b)) & 63
		d = b << s
		v = reciprocalNewton(d)
	}

	// The dividend scaled, three words, and the places k of the scale. lt
	// is 1 when the leading ratio is below one.
	var n2, n1, n0 uint64
	var k int
	if xhi != 0 {
		// A dividend of two words is the longer coefficient, by e
		// digits, and the divisor scaled to its length still fits two
		// words. The scale of the dividend is then at most the digits of
		// the divisor; one beyond the word powers is left to divLong128,
		// as is a coefficient that is not canonical.
		xc := Uint128{Hi: xhi, Lo: a}
		e := cDigits128(xc) - db
		t := &pow10tab128[e]
		bh, bl := bits.Mul64(b, t.Lo)
		bh += b * t.Hi
		_, lt := bits.Sub64(a, bl, 0)
		_, lt = bits.Sub64(xhi, bh, lt)
		k = Precision128 - 1 - e + int(lt)
		if k >= len(pow10tab64) || u128GtMaxCoeff(xc) {
			return ubdDivLong128(x, y, mode)
		}
		p := pow10word[k&31]
		var h0, l1, c uint64
		h0, n0 = bits.Mul64(a, p)
		n2, l1 = bits.Mul64(xhi, p)
		n1, c = bits.Add64(h0, l1, 0)
		n2 += c
	} else {
		// Which coefficient of two words is the shorter depends on the
		// operands as much as their digits do, so both are scaled, one
		// of them by 10^0, and nothing branches.
		e := db - wordDigits(a)
		sign := e >> intSign // all ones when x is the longer
		ah, al := bits.Mul64(a, pow10word[(e&^sign)&31])
		bh, bl := bits.Mul64(b, pow10word[(-e&sign)&31])
		_, lt := bits.Sub64(al, bl, 0)
		_, lt = bits.Sub64(ah, bh, lt)
		k = Precision128 - 1 + e + int(lt)

		// Past the last 128-bit power, a takes the excess: it has at
		// most db - 4 digits then.
		m, p := a, k
		if k >= len(pow10tab128) {
			p = len(pow10tab128) - 1
			m = a * pow10word[(k-p)&31]
		}
		t := &pow10tab128[p]
		var h0, l1, c uint64
		h0, n0 = bits.Mul64(m, t.Lo)
		n2, l1 = bits.Mul64(m, t.Hi)
		n1, c = bits.Add64(h0, l1, 0)
		n2 += c
	}
	half := b >> 1
	n0, c := bits.Add64(n0, half, 0)
	n1, c = bits.Add64(n1, 0, c)
	n2 += c

	// The quotient is below 2^113, so n2 is below b >> 15: the
	// precondition of each form of the first step.
	var q1, q0, rem uint64
	if div64Native {
		q1, rem = bits.Div64(n2, n1, b)
		q0, rem = bits.Div64(rem, n0, b)
	} else if small {
		// The top 96 bits of the dividend are below 2^49, and each
		// remainder with the next half word is below 2^64.
		w := n2<<32 | n1>>32
		q3 := w / b
		w = (w-q3*b)<<32 | n1&(1<<32-1)
		q2 := w / b
		w = (w-q2*b)<<32 | n0>>32
		q1 = q3<<32 | q2
		q3 = w / b
		w = (w-q3*b)<<32 | n0&(1<<32-1)
		q2 = w / b
		q0 = q3<<32 | q2
		rem = w - q2*b
	} else {
		u2 := n2<<s | n1>>1>>((63-s)&63)
		u1 := n1<<s | n0>>1>>((63-s)&63)
		q1, rem = div2by1(u2, u1, d, v)
		q0, rem = div2by1(rem, n0<<s, d, v)
		rem >>= s
	}
	if rem == half {
		// Exact, and not found so above (a dividend of two words, or a
		// quotient too long for a word): the zeros that the scale put
		// in go, as far as the preferred exponent.
		first := 0
		if k < Precision128 {
			first = k
		}
		var q Uint128
		q, k = strip128(Uint128{Hi: q1, Lo: q0}, k, first)
		q1, q0 = q.Hi, q.Lo
	} else if rem == 0 && b&1 == 0 && mode != ToNearestAway {
		q0 &^= 1 // a tie: the even neighbour is the one below
	}
	if r, ok := ubdPackWord(neg, q1, q0, exp-k); ok {
		return r
	}
	return ubdDivLong128(x, y, mode)
}

func ubdDivWide128(x, y UBD128, mode Mode) UBD128 {
	xhi, yhi, exp, neg, _ := ubdDivPair128(x, y)
	xc, yc := Uint128{Hi: xhi, Lo: x.Lo}, Uint128{Hi: yhi, Lo: y.Lo}
	dx, dy := cDigits128(xc), cDigits128(yc)
	// A zero dividend has no digits and a coefficient that is not
	// canonical has 35.
	if uint(dx-1) >= Precision128 || dy > Precision128 {
		return ubdDivLong128(x, y, mode)
	}
	// The digit counts come before the reciprocal in the text because
	// they start the longer chain: what is written first is started
	// first, and the reciprocal has time to spare.
	s := uint(bits.LeadingZeros64(yhi)) & 63 // at least 15: yhi is below 2^49
	v1 := yhi<<s | y.Lo>>((64-s)&63)
	v0 := y.Lo << s
	vv := reciprocalExtend(reciprocal2by1(v1), v1, v0)

	// lt is 1 when the leading ratio is below one.
	e := dy - dx
	sign := e >> intSign // all ones when x is the longer
	px, py := &pow10tab128[e&^sign], &pow10tab128[-e&sign]
	sh, sl := bits.Mul64(xc.Lo, px.Lo)
	sh += xc.Lo*px.Hi + xc.Hi*px.Lo
	th, tl := bits.Mul64(yc.Lo, py.Lo)
	th += yc.Lo*py.Hi + yc.Hi*py.Lo
	_, lt := bits.Sub64(sl, tl, 0)
	_, lt = bits.Sub64(sh, th, lt)

	// The dividend x × 10^k with k = k1 + lt. The scale is started from
	// k1, which the digit counts give, and does not wait for the leading
	// ratio: m = x × 10^kk, with kk the places of k1 beyond the last
	// 128-bit power, has at most dy - 5 digits, takes the one more place
	// of lt as shifts and a mask, and is then multiplied by the power of
	// the rest.
	const last = len(pow10tab128) - 1
	k1 := Precision128 - 1 + e
	kk := k1 - last
	kk &^= kk >> intSign
	pm, pt := &pow10tab128[kk], &pow10tab128[k1-kk]
	mh, ml := bits.Mul64(xc.Lo, pm.Lo)
	mh += xc.Lo*pm.Hi + xc.Hi*pm.Lo
	// m × 10 = m × 8 + m × 2, under the mask of lt.
	m8h, m8l := (mh<<3|ml>>61)&-lt, (ml<<3)&-lt
	m2h, m2l := (mh<<1|ml>>63)&-lt, (ml<<1)&-lt
	mh, ml = mh&(lt-1), ml&(lt-1)
	ml, c := bits.Add64(ml, m8l, 0)
	mh += m8h + c
	ml, c = bits.Add64(ml, m2l, 0)
	mh += m2h + c
	k := k1 + int(lt)
	h00, n0 := bits.Mul64(ml, pt.Lo)
	h01, l01 := bits.Mul64(ml, pt.Hi)
	h10, l10 := bits.Mul64(mh, pt.Lo)
	h11, l11 := bits.Mul64(mh, pt.Hi)
	n1, c1 := bits.Add64(h00, l01, 0)
	n1, c2 := bits.Add64(n1, l10, 0)
	n2, c1 := bits.Add64(h01, h10, c1)
	n2, c2 = bits.Add64(n2, l11, c2)
	n3 := h11 + c1 + c2

	// Biased by half the divisor, and shifted with it.
	half := Uint128{Hi: yhi >> 1, Lo: y.Lo>>1 | yhi<<63}
	n0, c = bits.Add64(n0, half.Lo, 0)
	n1, c = bits.Add64(n1, half.Hi, c)
	n2, c = bits.Add64(n2, 0, c)
	n3 += c
	r := (64 - s) & 63
	// Two of div3by2's steps (u256.go), written out: as calls they were
	// 25 instructions and about 4 cycles of each 34-digit division, for
	// the registers the calls send through memory.
	var q1, q0, r1, r0 uint64
	{
		u2, u1, u0 := n3<<s|n2>>r, n2<<s|n1>>r, n1<<s|n0>>r
		q, qf := bits.Mul64(vv, u2)
		qf, c := bits.Add64(qf, u1, 0)
		q += u2 + c
		t1, t0 := bits.Mul64(v0, q)
		p0, c := bits.Sub64(u0, t0, 0)
		p1, _ := bits.Sub64(u1-q*v1, t1, c)
		r0, c = bits.Sub64(p0, v0, 0)
		r1, _ = bits.Sub64(p1, v1, c)
		// (p1, p0) is the remainder against q and (r1, r0) the one
		// against q + 1: both are formed and one is selected.
		qn := q + 1
		if r1 >= qf {
			qn, r1, r0 = q, p1, p0
		}
		q = qn
		if r1 > v1 || r1 == v1 && r0 >= v0 {
			q++
			r0, c = bits.Sub64(r0, v0, 0)
			r1, _ = bits.Sub64(r1, v1, c)
		}
		q1 = q
	}
	{
		u2, u1, u0 := r1, r0, n0<<s
		q, qf := bits.Mul64(vv, u2)
		qf, c := bits.Add64(qf, u1, 0)
		q += u2 + c
		t1, t0 := bits.Mul64(v0, q)
		p0, c := bits.Sub64(u0, t0, 0)
		p1, _ := bits.Sub64(u1-q*v1, t1, c)
		r0, c = bits.Sub64(p0, v0, 0)
		r1, _ = bits.Sub64(p1, v1, c)
		qn := q + 1
		if r1 >= qf {
			qn, r1, r0 = q, p1, p0
		}
		q = qn
		if r1 > v1 || r1 == v1 && r0 >= v0 {
			q++
			r0, c = bits.Sub64(r0, v0, 0)
			r1, _ = bits.Sub64(r1, v1, c)
		}
		q0 = q
	}
	rem := Uint128{Hi: r1 >> s, Lo: r0>>s | r1<<r}
	if rem == half {
		first := 0
		if k < Precision128 {
			first = k
		}
		var q Uint128
		q, k = strip128(Uint128{Hi: q1, Lo: q0}, k, first)
		q1, q0 = q.Hi, q.Lo
	} else if rem == (Uint128{}) && y.Lo&1 == 0 && mode != ToNearestAway {
		q0 &^= 1
	}
	if r, ok := ubdPackWord(neg, q1, q0, exp-k); ok {
		return r
	}
	return ubdDivLong128(x, y, mode)
}

func ubdDivLong128(x, y UBD128, mode Mode) UBD128 {
	xs, ys := x.Hi>>60 == UBDSpecialTag, y.Hi>>60 == UBDSpecialTag
	neg := (x.Hi^y.Hi)&UBDSignHi != 0
	if xs || ys {
		switch {
		case x.Hi>>59 == UBDNaNTag || y.Hi>>59 == UBDNaNTag:
			return ubdNaN2(x, y)
		case xs:
			if ys {
				// ∞ / ∞ is invalid (IEEE 754-2019 clause 7.2 item e).
				return UBD128{Hi: UBDNaNHi}
			}
			return ubdInf(neg)
		}
		// x / ∞ is a zero at MinExp128 ("Exponent of a result").
		return ubdRound128(neg, wide128{}, MinExp128, classZero, mode)
	}
	xc, xe := coef128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}, int(int64(x.Hi)>>UBDExpShift)
	yc, ye := coef128{Hi: y.Hi & UBDCoeffMaskHi, Lo: y.Lo}, int(int64(y.Hi)>>UBDExpShift)
	switch {
	case cIsZero128(yc):
		if cIsZero128(xc) {
			// 0 / 0 is invalid (clause 7.2 item e).
			return UBD128{Hi: UBDNaNHi}
		}
		// Division by zero (clause 7.3).
		return ubdInf(neg)
	case cIsZero128(xc):
		// A zero quotient takes the preferred exponent, clamped by ubdRound128.
		return ubdRound128(neg, wide128{}, xe-ye, classZero, mode)
	}

	dy := cDigits128(yc)
	k, q, class := dy-1, xc, classZero
	first := 1 // the likely count of zeros to strip from an exact quotient
	if !cEq128(yc, cPow10128(k)) {
		// Scale so that q has exactly Precision128 digits: xc × 10^k / yc
		// lies in [10^(dx+k-dy-1), 10^(dx+k-dy+1)) and reaches 10^(dx+k-dy)
		// exactly when the leading ratio does, xc × 10^(dy-dx) >= yc, a
		// comparison of two values of dy or dx digits, which fit.
		//
		// Which coefficient is the longer is a property of the operands'
		// kind and stays a branch; whether the ratio reaches one is in
		// their leading digits, as good as random, and is a borrow
		// (cLt128), not a branch.
		dx := cDigits128(xc)
		a, b := xc, yc
		if dx <= dy {
			a = cMulPow10128(xc, dy-dx)
		} else {
			b = cMulPow10128(yc, dx-dy)
		}
		lead := 1
		if cLt128(a, b) {
			lead = 0
		}
		k = Precision128 - dx + dy - lead
		n := cMulPow10Full128(xc, k)
		exp := xe - ye - k
		face := MinExp128 < exp && exp <= MaxExp128 && exp+Precision128-1 >= Emin128
		var rem coef128
		if face && (mode <= ToNearestAway || mode > TowardPositive) {
			// To nearest, inside the exponent range of roundFace128's proof:
			// the dividend is biased by half the divisor, and the quotient
			// of n + ⌊yc/2⌋ by yc is the quotient of n by yc rounded to
			// nearest with ties up, because the bias carries into the next
			// unit exactly when the fraction is a half or more. Its
			// remainder says the rest: it is ⌊yc/2⌋ exactly when yc divides
			// n, the exact quotient, which goes on to the preferred
			// exponent below; and it is zero with an even yc exactly on a
			// tie, where ties to even takes the even neighbour, the one
			// below. Nothing here waits for a comparison of the remainder
			// with the half or branches on the direction, which is as good
			// as random: the two tests are rare, and roundFace128, given no
			// class, only encodes.
			half := cHalf128(yc)
			nb, _ := cAddFull128(n, wide128{Lo: half})
			var rb coef128
			q, rb = cDivFull128(nb, yc)
			if !cEq128(rb, half) {
				if cIsZero128(rb) && !cOdd128(yc) && mode != ToNearestAway {
					q = cEven128(q)
				}
				return ubdRoundFace128(neg, q, exp, classZero, mode)
			}
		} else {
			q, rem = cDivFull128(n, yc)
		}
		if !cIsZero128(rem) {
			// An inexact quotient in a directed mode or near the exponent
			// limits: the remainder places the true value between q and
			// q + 1 at exponent xe - ye - k, and its class is exact: rem
			// against yc - rem. Within the exponent range that roundFace128's
			// proof names, that is the whole rounding decision, taken at
			// face value; nearer the limits round128 takes the usual round
			// digit and sticky bit, found by one more division step.
			other, _ := cSub128(yc, rem)
			class = cClass128(rem, other)
			if face {
				return ubdRoundFace128(neg, q, exp, class, mode)
			}
			d, rem2 := cDivFull128(wide128{Lo: cMulPow10128(rem, 1)}, yc)
			q, _ = cAdd128(cMulPow10128(q, 1), d)
			k++
			class = classZero
			if !cIsZero128(rem2) {
				class = classBelowHalf
			}
		} else if k < Precision128 {
			// The long division's exact quotient has all k zeros whenever
			// yc divides xc, the usual exact division (8.00 / 2): try them
			// at once. q has Precision128 digits, so k >= Precision128 cannot
			// strip whole; the search takes those.
			first = k
		} else {
			first = 0
		}
	}
	if class == classZero && k > 0 {
		// strip128's steps, written here and not called: as a call they
		// cost a decimal64 division with an exact quotient a sixth more,
		// and one that is not exact a twentieth, for what the call does
		// to this function's registers. div128's own paths, which reach
		// an exact quotient more rarely, call strip128.
		search := true
		if first > 0 {
			if sq, ok := cDivExactPow10128(q, first); ok {
				q, k = sq, k-first
				search = first == 1
			} else {
				search = first > 1
			}
		}
		if search && k > 0 {
			step := 1
			for step*2 <= min(k, Precision128+1) {
				step *= 2
			}
			for ; step > 0; step /= 2 {
				if step > k {
					continue
				}
				if sq, ok := cDivExactPow10128(q, step); ok {
					q, k = sq, k-step
				}
			}
		}
	}
	return ubdRound128(neg, wide128{Lo: q}, xe-ye-k, class, mode)
}
