// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv

import (
	"internal/bid"
	istrconv "internal/strconv"
	"math/bits"
)

// The fast path of the 'x' format, in the shape ParseFloat's is: one
// product of the coefficient and a 128-bit power of ten, with a proven
// bound on its error, which decides the digits unless a rounding boundary
// lies within that bound; the exact path of decimal_hex.go decides the rest.
//
// The table holds D, 10^p × 2^b rounded up to 128 bits, for p from -348 to
// 347 (internal/strconv's Pow10Ceil): 10^p × 2^b is in (D-1, D]. For a
// coefficient c below 2^128 the scaled value c × 10^p × 2^b is therefore in
// (X-c, X] with X = c × D, an interval narrower than c. If no rounding
// boundary of the digits asked for lies in it, every value in it rounds the
// same way, the true one included; otherwise the fast path gives up. The
// interval is open below and closed above, so a boundary at X itself, which
// an exact table entry can put there, also makes it give up: the exact path
// decides ties.

// hexPathHook, if set (by tests), is called with the route that decided a
// value: the product, the exact binary value, or the exact path of
// decimal_hex.go.
var hexPathHook func(path int)

const (
	hexPathProduct = iota
	hexPathBinary
	hexPathExact
)

// u256 is a 256-bit natural number, least significant word first.
type u256 [4]uint64

// mul128 returns (a1×2^64 + a0) × (b1×2^64 + b0).
func mul128(a1, a0, b1, b0 uint64) u256 {
	h00, l00 := bits.Mul64(a0, b0)
	h01, l01 := bits.Mul64(a0, b1)
	h10, l10 := bits.Mul64(a1, b0)
	h11, l11 := bits.Mul64(a1, b1)
	var r u256
	var c uint64
	r[0] = l00
	r[1], c = bits.Add64(h00, l01, 0)
	r[2], c = bits.Add64(h01, l11, c)
	r[3] = h11 + c
	r[1], c = bits.Add64(r[1], l10, 0)
	r[2], c = bits.Add64(r[2], h10, c)
	r[3] += c
	return r
}

// sub128 returns x - (h×2^64 + l), for an x at least that.
func (x u256) sub128(h, l uint64) u256 {
	var b uint64
	x[0], b = bits.Sub64(x[0], l, 0)
	x[1], b = bits.Sub64(x[1], h, b)
	x[2], b = bits.Sub64(x[2], 0, b)
	x[3], _ = bits.Sub64(x[3], 0, b)
	return x
}

func (x u256) bitLen() int {
	for i := 3; i >= 0; i-- {
		if x[i] != 0 {
			return i*64 + bits.Len64(x[i])
		}
	}
	return 0
}

// shr returns the low 128 bits of x / 2^k.
func (x u256) shr(k uint) (hi, lo uint64) {
	w, b := int(k/64), k%64
	word := func(i int) uint64 {
		if i < 4 {
			return x[i]
		}
		return 0
	}
	lo, hi = word(w), word(w+1)
	if b > 0 {
		lo = lo>>b | hi<<(64-b)
		hi = hi>>b | word(w+2)<<(64-b)
	}
	return hi, lo
}

// shl128 returns (hi×2^64 + lo) × 2^k and whether it fits in 256 bits.
func shl128(hi, lo uint64, k int) (u256, bool) {
	n := bits.Len64(lo)
	if hi != 0 {
		n = 64 + bits.Len64(hi)
	}
	if k < 0 || k+n > 256 {
		return u256{}, false
	}
	var r u256
	w, b := k/64, uint(k%64)
	src := [3]uint64{lo, hi, 0}
	for i := 0; i < 3 && w+i < 4; i++ {
		v := src[i] << b
		if b > 0 && i > 0 {
			v |= src[i-1] >> (64 - b)
		}
		r[w+i] = v
	}
	return r, true
}

func (x u256) cmp(y u256) int {
	for i := 3; i >= 0; i-- {
		if x[i] != y[i] {
			if x[i] < y[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// scaleFast returns c × D(p) and c × D(p) - c, the interval (lo, hi] that
// c × 10^p × 2^b lies in, and b; ok is false outside the table.
func scaleFast(c1, c0 uint64, p int) (hi, lo u256, b int, ok bool) {
	d1, d0, b, ok := istrconv.Pow10Ceil(p)
	if !ok {
		return
	}
	hi = mul128(c1, c0, d1, d0)
	return hi, hi.sub128(c1, c0), b, true
}

// hexRoundFast is hexRound for a value whose scaled value lies in (y, x]
// (from scaleFast, with its b), if that decides it: m (in mhi, mlo) of 4n+1
// bits and the exponent e of its leading bit, the value rounded to nearest,
// ties to even.
func hexRoundFast(x, y u256, b, n int) (mhi, mlo uint64, e int, ok bool) {
	l := x.bitLen()
	nb := 4*n + 1 // the bits kept
	d := l - nb   // the bits dropped
	// The half-units below take nb+1 bits, and one more for the carry of
	// rounding up; all in two words.
	if y.bitLen() != l || d < 1 || nb+2 > 128 {
		return 0, 0, 0, false
	}
	// The half-units of the last bit kept below the value, from either end
	// of the interval: if they differ, a boundary is in it.
	jh, jl := x.shr(uint(d - 1))
	yh, yl := y.shr(uint(d - 1))
	if jh != yh || jl != yl {
		return 0, 0, 0, false
	}
	// The value is strictly between the half-units j and j+1: below the
	// midpoint for an even j, above it for an odd one, so the rounded value
	// is (j+1)/2.
	var carry uint64
	jl, carry = bits.Add64(jl, 1, 0)
	jh += carry
	mlo, mhi = jl>>1|jh<<63, jh>>1
	e = l - 1 - b
	if mhi != 0 && bits.Len64(mhi)+64 > nb || mhi == 0 && bits.Len64(mlo) > nb {
		// 1.ff…f rounded up is 10.0…0.
		mlo, mhi = mlo>>1|mhi<<63, mhi>>1
		e++
	}
	return mhi, mlo, e, true
}

// exactBinary returns the value c × 10^q (c = c1×2^64 + c0) as m × 2^e2
// exactly, with m below 2^128, if it has such a form: for q >= 0 when
// c × 5^q fits, and for q < 0 when 5^-q divides c. Such a value lies on a
// binary boundary, where the product, which cannot tell it from a value
// just beside it, would always give up; integers and amounts such as 1.5
// are of this kind.
func exactBinary(c1, c0 uint64, q int) (m1, m0 uint64, e2 int, ok bool) {
	if q >= 0 {
		for range q {
			h, l := bits.Mul64(c0, 5)
			hh, lh := bits.Mul64(c1, 5)
			var carry uint64
			lh, carry = bits.Add64(lh, h, 0)
			if hh != 0 || carry != 0 {
				return 0, 0, 0, false
			}
			c1, c0 = lh, l
		}
		return c1, c0, q, true
	}
	k := -q
	if k > 48 { // 5^49 > 2^113 > c
		return 0, 0, 0, false
	}
	for k > 0 {
		step := min(k, 27)
		k -= step
		d := uint64pow5[step]
		qh, r := bits.Div64(0, c1, d)
		ql, r := bits.Div64(r, c0, d)
		if r != 0 {
			return 0, 0, 0, false
		}
		c1, c0 = qh, ql
	}
	return c1, c0, q, true
}

// hexRoundExact is hexRound for the exact value m × 2^e2 (m not zero).
func hexRoundExact(m1, m0 uint64, e2, n int) (mhi, mlo uint64, e int, ok bool) {
	l := bits.Len64(m0)
	if m1 != 0 {
		l = 64 + bits.Len64(m1)
	}
	nb := 4*n + 1
	if nb+1 > 128 {
		return 0, 0, 0, false
	}
	e = l - 1 + e2
	if l <= nb {
		x, _ := shl128(m1, m0, nb-l)
		return x[1], x[0], e, true
	}
	d := uint(l - nb) // bits dropped, at least 1
	x := u256{m0, m1}
	kh, kl := x.shr(d)
	// The dropped bits, rest, against half a unit, 2^(d-1).
	var restHi, restLo, halfHi, halfLo uint64
	switch {
	case d < 64:
		restLo = m0 & (1<<d - 1)
	case d == 64:
		restLo = m0
	default:
		restHi, restLo = m1&(1<<(d-64)-1), m0
	}
	if d-1 < 64 {
		halfLo = 1 << (d - 1)
	} else {
		halfHi = 1 << (d - 1 - 64)
	}
	above := restHi > halfHi || restHi == halfHi && restLo > halfLo
	tie := restHi == halfHi && restLo == halfLo
	if above || tie && kl&1 != 0 {
		kh, kl = add128(kh, kl, 1)
		if kh != 0 && bits.Len64(kh)+64 > nb || kh == 0 && bits.Len64(kl) > nb {
			kl, kh = kl>>1|kh<<63, kh>>1
			e++
		}
	}
	return kh, kl, e, true
}

// hexShortestFast is hexShortest decided by products, if they decide it.
// Each bound of the interval that reads back as the value is a product too,
// and a candidate is compared with the interval that bound is known to lie
// in: inside it, the fast path gives up.
func hexShortestFast(c1, c0 uint64, q, bitSize int) (mhi, mlo uint64, e, n, path int, ok bool) {
	prec, minExp, maxN := bid.Precision128, bid.MinExp128, 31
	if bitSize == 64 {
		prec, minExp, maxN = bid.Precision64, bid.MinExp64, 16
	}
	// The coefficient with every digit of the format, and its exponent.
	shift := prec - digits128(c1, c0)
	if q-shift < minExp {
		shift = q - minExp
	}
	cp := pow10tab128[shift]
	cw := mul128(c1, c0, cp[1], cp[0]) // below 10^34
	cc1, cc0 := cw[1], cw[0]
	qq := q - shift
	p := pow10tab128[prec-1]
	powerOfTen := cc1 == p[1] && cc0 == p[0] && qq > minExp

	// The bounds, times 2^(b+1): (2C+1) × 10^Q / 2 above, and
	// (2C-1) × 10^Q / 2 below, or (20C-1) × 10^(Q-1) / 2 below a power of ten.
	t1, t0 := mul128by(cc1, cc0, 2)
	u1, u0 := add128(t1, t0, 1)
	upHi, upLo, upB, ok1 := scaleFast(u1, u0, qq)
	var lowHi, lowLo u256
	var lowB int
	var ok2 bool
	if powerOfTen {
		w1, w0 := mul128by(cc1, cc0, 20)
		w1, w0 = sub128by1(w1, w0)
		lowHi, lowLo, lowB, ok2 = scaleFast(w1, w0, qq-1)
	} else {
		w1, w0 := sub128by1(t1, t0)
		lowHi, lowLo, lowB, ok2 = scaleFast(w1, w0, qq)
	}
	if !ok1 || !ok2 {
		return 0, 0, 0, 0, 0, false
	}
	// The candidates: exact if the value is a binary fraction, otherwise
	// from its product.
	b1, b0, e2, dyadic := exactBinary(c1, c0, q)
	x, y, b, ok3 := scaleFast(c1, c0, q)
	if !dyadic && !ok3 {
		return 0, 0, 0, 0, 0, false
	}

	// No n below nStart can give a candidate in the interval. A candidate
	// g in it is within half a unit of the coefficient C of v (δ = v/2C),
	// so |X - g| < δ + c with the product's error c, and with X, δ and c at
	// one scale, 2^p > δ + c. If g is a multiple of 2^d (d the bits n
	// drops), the bits of X from p to d are then all zeros or all ones. So
	// n fails while d - p exceeds the longer of the runs of zeros and of
	// ones at the bottom of X / 2^p.
	nStart := 0
	if !dyadic {
		l := x.bitLen()
		p := max(l-bitLen128(cc1, cc0)+2, bitLen128(c1, c0)) + 1
		r := x.shr256(uint(p))
		run := max(r.trailingZeros(), r.trailingOnes())
		if k := l - 1 - p - run; k > 0 {
			nStart = min((k+3)/4, maxN)
		}
	}

	for n := nStart; n <= maxN; n++ {
		var mhi, mlo uint64
		var e int
		var ok bool
		if dyadic {
			mhi, mlo, e, ok = hexRoundExact(b1, b0, e2, n)
		} else {
			mhi, mlo, e, ok = hexRoundFast(x, y, b, n)
		}
		if !ok {
			return 0, 0, 0, 0, 0, false
		}
		// The candidate m × 2^(e-4n), times 2^(b+1) of each bound.
		hu, ok1 := shl128(mhi, mlo, e-4*n+upB+1)
		hl, ok2 := shl128(mhi, mlo, e-4*n+lowB+1)
		if !ok1 || !ok2 {
			return 0, 0, 0, 0, 0, false
		}
		// Below the upper bound: at most its interval's lower end. Above
		// it: beyond its upper end. Between, which includes a candidate at
		// the bound itself, whose tie the coefficient's parity decides:
		// undecided, for the exact path.
		var below, above bool
		switch {
		case hu.cmp(upLo) <= 0:
			below = true
		case hu.cmp(upHi) > 0:
			below = false
		default:
			return 0, 0, 0, 0, 0, false
		}
		switch {
		case hl.cmp(lowHi) > 0:
			above = true
		case hl.cmp(lowLo) <= 0:
			above = false
		default:
			return 0, 0, 0, 0, 0, false
		}
		if below && above {
			path := hexPathProduct
			if dyadic {
				path = hexPathBinary
			}
			return mhi, mlo, e, n, path, true
		}
	}
	return 0, 0, 0, 0, 0, false
}

// shr256 returns x / 2^k.
func (x u256) shr256(k uint) u256 {
	var r u256
	w, b := int(k/64), k%64
	for i := 0; i+w < 4; i++ {
		r[i] = x[i+w] >> b
		if b > 0 && i+w+1 < 4 {
			r[i] |= x[i+w+1] << (64 - b)
		}
	}
	return r
}

// trailingZeros returns the number of zero bits below the lowest one, 256
// for zero.
func (x u256) trailingZeros() int {
	for i, w := range x {
		if w != 0 {
			return i*64 + bits.TrailingZeros64(w)
		}
	}
	return 256
}

// trailingOnes returns the number of one bits below the lowest zero.
func (x u256) trailingOnes() int {
	return u256{^x[0], ^x[1], ^x[2], ^x[3]}.trailingZeros()
}

func bitLen128(h, l uint64) int {
	if h != 0 {
		return 64 + bits.Len64(h)
	}
	return bits.Len64(l)
}

func mul128by(h, l, m uint64) (uint64, uint64) {
	hl, ll := bits.Mul64(l, m)
	return h*m + hl, ll
}

func add128(h, l, a uint64) (uint64, uint64) {
	l, c := bits.Add64(l, a, 0)
	return h + c, l
}

func sub128by1(h, l uint64) (uint64, uint64) {
	l, b := bits.Sub64(l, 1, 0)
	return h - b, l
}

// pow10tab128[k] is 10^k as {lo, hi}, for k up to 38.
var pow10tab128 = func() (t [39][2]uint64) {
	var h, l uint64 = 0, 1
	for k := range t {
		t[k] = [2]uint64{l, h}
		h, l = mul128by(h, l, 10)
	}
	return
}()

// digits128 returns the number of decimal digits of h×2^64 + l, at least 1,
// for a value below 10^38.
func digits128(h, l uint64) int {
	n := bitLen128(h, l) * 1233 >> 12 // log10(2) rounded down, to within 1e-4
	if p := pow10tab128[n]; h > p[1] || h == p[1] && l >= p[0] {
		n++
	}
	return max(n, 1)
}
