// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// div128 is Div128. A finite dividend over a finite divisor whose
// coefficient fits a word, neither of them zero, in a mode that rounds to
// nearest (amounts, rates and counts as they are usually held, and what a
// division left, divided again) is divided here, and every other operand,
// and a result outside the range that packWord128 encodes, goes to
// divLong128, which decides it from the start.
//
// The method is divLong128's (see divLongW) in the arithmetic that such
// operands need, and nothing is decoded twice, called or kept in memory on
// the way. The leading ratio compares the coefficients scaled to one
// length: for a dividend of one word as two products of two words, and for
// a dividend of two words, which is the longer, against the divisor times
// a power of ten. The dividend x × 10^k is three words, because the
// quotient is below 10^34 and the divisor below 2^64: a word times a power
// of two words (beyond the last 128-bit power the coefficient takes the
// excess first and is still a word), or two words times a power of one.
//
// Three words are divided by one in the way the processor makes cheapest.
// With an instruction that divides two words by one, that is two of them.
// Without it, a divisor below 2^32 divides half a word at a time by the
// instruction that divides a word by a word, four times: a chain of
// divisions, but a tenth of the work of the reciprocal, and the divisors of
// money are that small. A larger divisor takes its reciprocal
// (reciprocalNewton) and two steps by it (div2by1).
//
// As in divLongW the dividend is biased by half the divisor, so that the
// quotient is rounded half up and the remainder says the rest: equal to
// the half exactly when the division is exact, when the quotient goes to
// strip128 for its preferred exponent, and zero under an even divisor
// exactly on a tie. The rounded quotient cannot reach 10^34: it would need
// b × 10^34 - x × 10^k, a positive multiple of 10^min(k, 34), to be at most
// b / 2. A power of ten above b / 2 rules that out, and k is below the
// digits of b only for a dividend of 34 digits whose leading ratio reaches
// one, when the difference is at least 10^34 times b less the power of ten
// below it, and b is not that power. packWord128 checks
// the coefficient all the same, and the exponent, which is the only thing
// that can send a quotient of 34 digits elsewhere: at an exponent in range
// it is normal, and rounding it once is the whole result.
//
// The compiler calls this function directly, so its parameters and result
// are a contract with it: see the end of "Inline fast paths" in the package
// documentation.
func div128(x, y Uint128, mode Mode) Uint128 {
	xhi, yhi, exp, neg, ok := divPair128(x, y)
	a, b := x.Lo, y.Lo
	if !ok || mode > ToNearestAway {
		return divLong128(x, y, mode)
	}
	if yhi != 0 {
		return divWide128(x, y, mode)
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
			if r, ok := packWord128(neg, 0, 0, exp); ok {
				return r
			}
		} else if a|xhi != 0 && !u128GtMaxCoeff(Uint128{Hi: xhi, Lo: a}) {
			return cInf128(neg)
		}
		return divLong128(x, y, mode)
	}
	db := wordDigits(b)
	if b == pow10word[(db-1)&31] {
		// A power of ten divides by moving the point.
		q, k := Uint128{Hi: xhi, Lo: a}, db-1
		if u128GtMaxCoeff(q) {
			return divLong128(x, y, mode)
		}
		if k > 0 {
			q, k = strip128(q, k, 1)
		}
		if r, ok := packWord128(neg, q.Hi, q.Lo, exp-k); ok {
			return r
		}
		return divLong128(x, y, mode)
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
					if r, ok := packWord128(neg, 0, lo/b, exp-j); ok {
						return r
					}
					return divLong128(x, y, mode)
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
			return divLong128(x, y, mode)
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
	if r, ok := packWord128(neg, q1, q0, exp-k); ok {
		return r
	}
	return divLong128(x, y, mode)
}

// divWide128 is div128 for a divisor whose coefficient has two words: both
// operands finite and of the first form, in a mode that rounds to nearest,
// which div128 has established. It is the method of div128 with a divisor
// of two words, and it hands the same cases on to divLong128. A divisor
// that is a power of ten is divided like any other: its quotient is exact
// and is stripped.
//
// Both coefficients are scaled to one length for the leading ratio, one of
// them by 10^0, each product within two words. The dividend x × 10^k has up
// to four words and k up to 67 places: the coefficient takes the places
// beyond the last 128-bit power first, staying within two words, and then
// that power. The divisor is normalised and the dividend shifted with it,
// which loses nothing because the quotient is below 2^113, and the
// quotient's two words are two steps of div3by2 by the reciprocal of the
// divisor.
//
// What bounds this function is the chain from the operands to the
// quotient more than the count of its instructions, so the chain is kept
// short: the scale starts from the digit counts and takes the place that
// the leading ratio decides at the end, on two words; and both steps use
// the reciprocal of both words, which costs more to prepare than that of
// the high word but is prepared beside the scale, and makes each step half
// as long. (Asking for the reciprocal before the digit counts, which
// leaves less to save across the calls, measured a tenth slower: the
// chain that matters then starts ninety instructions late.)
func divWide128(x, y Uint128, mode Mode) Uint128 {
	xhi, yhi, exp, neg, _ := divPair128(x, y)
	xc, yc := Uint128{Hi: xhi, Lo: x.Lo}, Uint128{Hi: yhi, Lo: y.Lo}
	dx, dy := cDigits128(xc), cDigits128(yc)
	// A zero dividend has no digits and a coefficient that is not
	// canonical has 35.
	if uint(dx-1) >= Precision128 || dy > Precision128 {
		return divLong128(x, y, mode)
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
	if r, ok := packWord128(neg, q1, q0, exp-k); ok {
		return r
	}
	return divLong128(x, y, mode)
}

// fives returns the number of times that 5 divides n, which is not zero,
// and n without those factors. A multiple of 5 times the inverse of 5
// modulo 2^64 is its quotient, at most ⌊(2^64-1)/5⌋, and every other
// value lands above that. The loop runs once for most coefficients and a
// few times for the rest.
func fives(n uint64) (c int, rest uint64) {
	const inv5, max5 = 0xCCCCCCCCCCCCCCCD, ^uint64(0) / 5
	for {
		m := n * inv5
		if m > max5 {
			return c, n
		}
		n, c = m, c+1
	}
}

// pow10word is pow10tab64 in an array of 32, so that an index masked to
// five bits needs no bound test; TestPow10Word compares the two.
var pow10word = [32]uint64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19,
}

// wordDigits returns the number of decimal digits of c, which is not zero.
// It is cDigits64 with the comparison as a borrow: whether c reaches the
// power of ten that its bit length suggests is in its leading digits, as
// good as random.
func wordDigits(c uint64) int {
	t := bits.Len64(c) * 1233 >> 12
	_, b := bits.Sub64(c, pow10word[t&31], 0)
	return t + 1 - int(b)
}
