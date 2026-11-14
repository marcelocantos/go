// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv

import (
	"internal/bid"
	"internal/decimal"
	"math/bits"
)

// This file writes the 'x' and 'X' formats of a decimal: the value as a
// hexadecimal fraction and a binary exponent, laid out as FormatFloat lays
// out a float's, -0x1.hhhhp±dd. The value of a decimal is c × 10^q, which is
// rarely a finite binary fraction, so the digits are those of the value
// rounded once, to nearest with ties to even, at the precision asked for.
// The precision -1 asks for the fewest digits that ParseDecimal reads back as
// the same value. Hexadecimal text has no quantum, so the value comes back
// and the quantum does not.

// appendDecimalX appends the 'x' or 'X' form of the decimal x of the given
// bitSize at precision prec.
func appendDecimalX(dst []byte, x bid.Uint128, fmt byte, prec, bitSize int) []byte {
	kind, neg, coeff, q := bid.Unpack128(x)
	switch kind {
	case bid.KindNaN:
		return append(dst, "NaN"...)
	case bid.KindInf:
		if neg {
			return append(dst, "-Inf"...)
		}
		return append(dst, "+Inf"...)
	}
	if coeff == (bid.Uint128{}) {
		return appendHexFloat(dst, neg, nil, 0, max(prec, 0), fmt)
	}
	c := nat{coeff.Lo, coeff.Hi}.norm()
	if prec >= 0 {
		m, e := hexRound(c, q, prec)
		return appendHexFloat(dst, neg, m, e, prec, fmt)
	}

	if m, e, n, ok := hexShortest(c, q, bitSize); ok {
		return appendHexFloat(dst, neg, m, e, n, fmt)
	}
	// hexShortest covers every value; this is the definition, kept as the
	// fallback: the fewest digits that ParseDecimal reads back as the value.
	var buf []byte
	for n := 0; ; n++ {
		m, e := hexRound(c, q, n)
		buf = appendHexFloat(buf[:0], neg, m, e, n, 'x')
		d, st := readDecimal(string(buf), bitSize)
		if st&(bid.Syntax|bid.Overflow) == 0 && hexSameValue(d, neg, c, q) {
			return appendHexFloat(dst, neg, m, e, n, fmt)
		}
	}
}

// hexShortest returns the 'x' digits of c × 10^q, which is not zero and is a
// value of the format of bitSize, at the fewest digits n after the point
// that ParseDecimal reads back as that value: m × 2^(e-4n), with m of 4n+1
// bits, rounded to nearest, ties to even. ok is false if no n up to the
// bound it tries does, which the bound rules out.
//
// ParseDecimal rounds the exact binary value of the text to the format, so
// the text reads back as the value exactly when the binary value lies in
// the interval of values that round to it: within half a unit in the last
// place on either side, where the unit is that of the coefficient written
// with every digit of the format, a tenth of it below a power of ten (whose
// neighbour below has a smaller exponent), and at a bound itself only if the
// coefficient is even. The bounds are worked out once, scaled by 2^s, as is
// the value, and each candidate is compared with them.
//
// Rounding to n digits is off by at most half of 2^(e-4n); half a unit of
// a normal value is at least 2^e / 10^p (p the precision), and below a power
// of ten a tenth of that, so every n with 4n > log2(10^p) reads back, which
// for decimal128 is n = 29 and for decimal64 n = 14. A subnormal value has a
// wider interval.
func hexShortest(c nat, q int, bitSize int) (nat, int, int, bool) {
	prec, minExp, maxN := bid.Precision128, bid.MinExp128, 31
	if bitSize == 64 {
		prec, minExp, maxN = bid.Precision64, bid.MinExp64, 16
	}

	// The coefficient with every digit of the format, and its exponent.
	shift := prec - len(c.decimal())
	if q-shift < minExp {
		shift = q - minExp
	}
	cc, qq := c.mulPow10(shift), q-shift
	powerOfTen := cc.cmp(nat{1}.mulPow10(prec-1)) == 0 && qq > minExp
	even := cc[0]&1 == 0

	// The value and the bounds, times 2^s: floor and whether the floor
	// dropped anything.
	const guard = 4
	lead := c.bitLen() - 1 + int(int64(q)*1741647>>19)
	s := 4*maxN + 1 + guard - lead
	t, tSticky := scaleBinary(c, q, s)
	e := t.bitLen() - 1 - s
	twice := cc.mulAdd(2, 0)
	hi, hiSticky := scaleBinary(twice.mulAdd(1, 1), qq, s-1) // (2C+1) × 10^Q / 2
	var lo nat
	var loSticky bool
	if powerOfTen {
		lo, loSticky = scaleBinary(cc.mulAdd(20, 0).sub1(), qq-1, s-1) // (20C-1) × 10^(Q-1) / 2
	} else {
		lo, loSticky = scaleBinary(twice.sub1(), qq, s-1) // (2C-1) × 10^Q / 2
	}

	for n := 0; n <= maxN; n++ {
		r := s - (4*n - e) // the bits of t below the last digit kept
		if r < 1 {
			break
		}
		u, st := t.shrSticky(uint(r - 1))
		st = st || tSticky
		roundBit := u[0]&1 != 0
		m, em := u.shr1(), e
		if roundBit && (st || m[0]&1 != 0) {
			m = m.mulAdd(1, 1)
			if m.bitLen() > 4*n+1 {
				m, em = m.shr1(), e+1
			}
		}
		h := m.shl(uint(s - (4*n - em))) // the candidate times 2^s, exactly
		// Below the upper bound, or at it with an even coefficient.
		ch := h.cmp(hi)
		below := ch < 0 || ch == 0 && (hiSticky || even)
		// Above the lower bound, or at it with an even coefficient.
		cl := h.cmp(lo)
		above := cl > 0 || cl == 0 && !loSticky && even
		if below && above {
			return m, em, n, true
		}
	}
	return nil, 0, 0, false
}

// hexSameValue reports whether the decimal d has the value (-1)^neg × c ×
// 10^q.
func hexSameValue(d decimal128, neg bool, c nat, q int) bool {
	hi, lo := decimal.BID128bits(d)
	kind, dneg, dc, dq := bid.Unpack128(bid.Uint128{Hi: hi, Lo: lo})
	if kind != bid.KindFinite || dneg != neg {
		return false
	}
	a := nat{dc.Lo, dc.Hi}.norm()
	b := c
	// Bring both to the smaller exponent; the coefficients are then equal
	// exactly when the values are.
	if dq > q {
		a = a.mulPow10(dq - q)
	} else if q > dq {
		b = b.mulPow10(q - dq)
	}
	return a.cmp(b) == 0
}

// hexRound returns c × 10^q, which is not zero, as m × 2^(e-4n): m has
// exactly 4n+1 bits, the leading one and 4n after the point, and is the
// value rounded to nearest, ties to even, at that many bits.
func hexRound(c nat, q int, n int) (nat, int) {
	// The binary exponent of the leading bit is lead or one of its
	// neighbours: c has bitLen(c) bits, and 1741647 / 2^19 is log2(10)
	// rounded down, to within 2e-7, which over the formats' range of q is
	// below a bit.
	lead := c.bitLen() - 1 + int(int64(q)*1741647>>19)
	// Work out floor(v × 2^s) for an s that gives a few bits more than the
	// 4n+2 needed (the leading bit, 4n after the point and a rounding bit),
	// with a sticky bit for the rest.
	const guard = 4
	s := 4*n + 1 + guard - lead
	t, sticky := scaleBinary(c, q, s)
	// Drop the bits beyond the rounding bit into the sticky bit.
	extra := t.bitLen() - (4*n + 2)
	if extra < 0 {
		panic("strconv: hexRound lost bits") // the guard covers the estimate
	}
	t, st := t.shrSticky(uint(extra))
	sticky = sticky || st
	s -= extra
	e := 4*n + 1 - s // the exponent of the leading bit

	round := t[0]&1 != 0
	m := t.shr1()
	if round && (sticky || m[0]&1 != 0) {
		m = m.mulAdd(1, 1)
		if m.bitLen() > 4*n+1 {
			// 1.ff…f + one unit is 10.00…0.
			m = m.shr1()
			e++
		}
	}
	return m, e
}

// scaleBinary returns floor(c × 10^q × 2^s) and whether anything was
// dropped by the floor. For a negative q the division by 10^-q is a shift
// by -q and a division by 5^-q, which leaves the dividend smaller and the
// divisions fewer.
func scaleBinary(c nat, q, s int) (nat, bool) {
	if q >= 0 {
		t := c.mulPow10(q)
		if s >= 0 {
			return t.shl(uint(s)), false
		}
		return t.shrSticky(uint(-s))
	}
	k := -q
	var t nat
	sticky := false
	if b := s - k; b >= 0 {
		t = c.shl(uint(b))
	} else {
		t, sticky = c.shrSticky(uint(-b))
	}
	t, st := t.divPow5(k)
	return t, sticky || st
}

// divPow5 returns floor(z / 5^k) and whether the remainder is not zero.
func (z nat) divPow5(k int) (nat, bool) {
	r := make(nat, len(z))
	copy(r, z)
	r = r.norm()
	sticky := false
	for k > 0 && len(r) > 0 {
		step := min(k, len(uint64pow5)-1)
		k -= step
		d := uint64pow5[step]
		var rem uint64
		for i := len(r) - 1; i >= 0; i-- {
			r[i], rem = bits.Div64(rem, r[i], d)
		}
		sticky = sticky || rem != 0
		r = r.norm()
	}
	if k > 0 {
		return nil, sticky || len(z.norm()) > 0
	}
	return r, sticky
}

// uint64pow5 holds 5^0 to 5^27, the largest power of five in a word.
var uint64pow5 = func() (p [28]uint64) {
	p[0] = 1
	for i := 1; i < len(p); i++ {
		p[i] = p[i-1] * 5
	}
	return
}()

// appendHexFloat lays out (-1)^neg × m × 2^(e-4n), with m of 4n+1 bits, or
// zero for an empty m, as FormatFloat's 'x' and 'X' do: "0x", the leading
// digit, prec digits after the point (n of them from m and zeros after),
// and a binary exponent of at least two digits.
func appendHexFloat(dst []byte, neg bool, m nat, e, prec int, fmt byte) []byte {
	hex := lowerhexDigits
	if fmt == 'X' {
		hex = upperhexDigits
	}
	if neg {
		dst = append(dst, '-')
	}
	lead := byte('0')
	if len(m) > 0 {
		lead = '1'
	} else {
		e = 0
	}
	dst = append(dst, '0', fmt, lead)
	n := (m.bitLen() - 1) / 4 // digits after the point in m
	if len(m) == 0 {
		n = 0
	}
	if prec > 0 {
		dst = append(dst, '.')
		for i := 0; i < prec; i++ {
			d := 0
			if i < n {
				d = int(m.bitsAt(4*(n-1-i), 4))
			}
			dst = append(dst, hex[d])
		}
	}
	if fmt == 'X' {
		dst = append(dst, 'P')
	} else {
		dst = append(dst, 'p')
	}
	if e < 0 {
		dst = append(dst, '-')
		e = -e
	} else {
		dst = append(dst, '+')
	}
	if e < 10 {
		dst = append(dst, '0')
	}
	return AppendInt(dst, int64(e), 10)
}

const (
	lowerhexDigits = "0123456789abcdef"
	upperhexDigits = "0123456789ABCDEF"
)

// bitsAt returns the k bits of z (k at most 64) from bit i up.
func (z nat) bitsAt(i, k int) uint64 {
	w, b := i/64, uint(i%64)
	var v uint64
	if w < len(z) {
		v = z[w] >> b
	}
	if b > 0 && w+1 < len(z) {
		v |= z[w+1] << (64 - b)
	}
	return v & (1<<k - 1)
}

// shr1 returns z / 2.
func (z nat) shr1() nat {
	r, _ := z.shrSticky(1)
	return r
}

// sub1 returns z - 1 for a z that is not zero.
func (z nat) sub1() nat {
	r := make(nat, len(z))
	copy(r, z)
	for i := range r {
		r[i]--
		if r[i] != ^uint64(0) {
			break
		}
	}
	return r.norm()
}

// cmp compares z and y.
func (z nat) cmp(y nat) int {
	z, y = z.norm(), y.norm()
	if len(z) != len(y) {
		if len(z) < len(y) {
			return -1
		}
		return 1
	}
	for i := len(z) - 1; i >= 0; i-- {
		if z[i] != y[i] {
			if z[i] < y[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
