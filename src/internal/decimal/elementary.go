// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal

import (
	"internal/bid"
	"math/bits"
)

// This file holds the functions of IEEE 754-2019 that are not arithmetic
// on coefficients: the square root, the scaling and neighbour operations
// of clause 5.3, and the powers, exponential and logarithms of clause
// 9.2. All of them are correctly rounded, to nearest with ties to even.
//
// A function here unpacks its operands, decides the special cases and
// the exact results from the coefficients, and for the rest computes the
// value in the multiple-precision arithmetic of mp.go and rounds it once
// with internal/bid's rounding routine. See ziv for how a result that is
// not exact is made certain before it is rounded.
//
// These functions are written to be right first. They were not tuned as
// the arithmetic of internal/bid was, but a first evaluation, which is
// nearly always the only one, allocates nothing (see arena in mp.go).

type u128 = bid.Uint128

// A dec is an unpacked decimal. A finite one is (-1)^neg × c × 10^e.
type dec struct {
	kind bid.Kind
	neg  bool
	c    u128
	e    int
}

// A result is a dec to be rounded to a format: for a finite one the exact
// value is (-1)^neg × (c + f) × 10^e, with f zero unless sticky is set,
// and then some fraction strictly between 0 and 1.
type result struct {
	dec
	sticky bool
}

// A format holds what the functions need to know of a width.
type format struct {
	p      int // digits of precision
	minExp int // least exponent
	maxExp int // greatest exponent
	words  int // words of mp precision for the first attempt
}

// The first attempt must hold the p+3 digits that are taken, the
// guardBits that may be wrong, and enough bits between the two that the
// test of mpDigits seldom fails. Three words do for both widths:
// decimal128's 37 digits are 123 bits, which with the guard leaves 25,
// so that about one evaluation in 2^24 is done twice.
var (
	format64  = format{bid.Precision64, bid.MinExp64, bid.MaxExp64, 3}
	format128 = format{bid.Precision128, bid.MinExp128, bid.MaxExp128, 3}
)

// farExp is an exponent beyond every format's range in either direction,
// small enough to add to another without overflow of a 32-bit int.
const farExp = 1 << 24

func clampExp(e int64) int {
	return int(min(max(e, -farExp), farExp))
}

// mulExp returns e × n, limited to ±2×farExp: an exponent times a count,
// which overflows nothing however large the count.
func mulExp(e int, n int64) int64 {
	if e == 0 || n == 0 {
		return 0
	}
	if n > farExp || n < -farExp {
		if (e < 0) != (n < 0) {
			return -2 * farExp
		}
		return 2 * farExp
	}
	return int64(e) * n
}

func unpack64(x uint64) dec {
	kind, neg, c, e := bid.Unpack64(x)
	return dec{kind, neg, u128{Lo: c}, e}
}

func unpack128(x u128) dec {
	kind, neg, c, e := bid.Unpack128(x)
	return dec{kind, neg, c, e}
}

func (r result) pack64() uint64 {
	switch r.kind {
	case bid.KindNaN:
		return bid.NaN64
	case bid.KindInf:
		if r.neg {
			return bid.Neg64(bid.Inf64)
		}
		return bid.Inf64
	}
	// The rounding routine takes one word. Keep 19 digits, which is at
	// least a digit more than the format's 16, and fold the rest into
	// sticky.
	c, e, sticky := r.c, int64(r.e), r.sticky
	if k := digitsU(c) - 19; k > 0 {
		var exact bool
		c, exact = divPow10U(c, k)
		sticky = sticky || !exact
		e += int64(k)
	}
	x, _ := bid.Round64(r.neg, c.Lo, clampExp(e), sticky, bid.ToNearestEven)
	return x
}

func (r result) pack128() u128 {
	switch r.kind {
	case bid.KindNaN:
		return u128{Hi: bid.NaN128Hi}
	case bid.KindInf:
		if r.neg {
			return bid.Neg128(u128{Hi: bid.Inf128Hi})
		}
		return u128{Hi: bid.Inf128Hi}
	}
	x, _ := bid.Round128(r.neg, r.c, r.e, r.sticky, bid.ToNearestEven)
	return x
}

func resNaN() result         { return result{dec: dec{kind: bid.KindNaN}} }
func resInf(neg bool) result { return result{dec: dec{kind: bid.KindInf, neg: neg}} }
func resZero(neg bool, e int64) result {
	return result{dec: dec{neg: neg, e: clampExp(e)}}
}
func resOne() result { return result{dec: dec{c: u128{Lo: 1}}} }

// exact returns the result for the exact value (-1)^neg × c × 10^e,
// written with the exponent nearest to pref that the format's precision
// allows, as IEEE 754-2019 asks of an exact result (clause 5.2).
func (f *format) exact(neg bool, c u128, e, pref int64) result {
	if c.Hi|c.Lo == 0 {
		return resZero(neg, pref)
	}
	if k := min(e-pref, int64(f.p-digitsU(c))); k > 0 {
		c, _ = mulU(c, pow10U(int(k)))
		e -= k
	}
	return result{dec: dec{neg: neg, c: c, e: clampExp(e)}}
}

// 128-bit integers.

var pow10Word = [20]uint64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19,
}

// pow10U returns 10^k, for k in [0, 38].
func pow10U(k int) u128 {
	if k < len(pow10Word) {
		return u128{Lo: pow10Word[k]}
	}
	hi, lo := bits.Mul64(pow10Word[19], pow10Word[k-19])
	return u128{Hi: hi, Lo: lo}
}

func cmpU(a, b u128) int {
	switch {
	case a.Hi != b.Hi:
		if a.Hi < b.Hi {
			return -1
		}
		return 1
	case a.Lo != b.Lo:
		if a.Lo < b.Lo {
			return -1
		}
		return 1
	}
	return 0
}

func isOneU(a u128) bool { return a.Hi == 0 && a.Lo == 1 }

func addU(a, b u128) u128 {
	lo, c := bits.Add64(a.Lo, b.Lo, 0)
	hi, _ := bits.Add64(a.Hi, b.Hi, c)
	return u128{Hi: hi, Lo: lo}
}

func subU(a, b u128) u128 {
	lo, c := bits.Sub64(a.Lo, b.Lo, 0)
	hi, _ := bits.Sub64(a.Hi, b.Hi, c)
	return u128{Hi: hi, Lo: lo}
}

// mulU returns a × b and whether it fits in 128 bits.
func mulU(a, b u128) (u128, bool) {
	if a.Hi != 0 && b.Hi != 0 {
		return u128{}, false
	}
	hi, lo := bits.Mul64(a.Lo, b.Lo)
	h1, l1 := bits.Mul64(a.Hi, b.Lo)
	h2, l2 := bits.Mul64(a.Lo, b.Hi)
	hi, c1 := bits.Add64(hi, l1, 0)
	hi, c2 := bits.Add64(hi, l2, 0)
	return u128{Hi: hi, Lo: lo}, h1|h2|c1|c2 == 0
}

// divWordU returns the quotient and remainder of a by d.
func divWordU(a u128, d uint64) (u128, uint64) {
	hi, r := a.Hi/d, a.Hi%d
	lo, r := bits.Div64(r, a.Lo, d)
	return u128{Hi: hi, Lo: lo}, r
}

// divPow10U returns a / 10^k truncated, for k in [0, 38], and whether
// the division was exact.
func divPow10U(a u128, k int) (u128, bool) {
	exact := true
	for k > 0 {
		j := min(k, 19)
		var r uint64
		a, r = divWordU(a, pow10Word[j])
		exact = exact && r == 0
		k -= j
	}
	return a, exact
}

func lenU(a u128) int {
	if a.Hi != 0 {
		return 64 + bits.Len64(a.Hi)
	}
	return bits.Len64(a.Lo)
}

// digitsU returns the number of decimal digits of a, and 0 for zero.
func digitsU(a u128) int {
	// 1233/4096 is just above log10(2) and exact enough for 128 bits:
	// the estimate is the digit count or one less.
	d := lenU(a) * 1233 >> 12
	if d < 39 && cmpU(a, pow10U(d)) >= 0 {
		d++
	}
	return d
}

// stripU returns a nonzero a without its trailing decimal zeros, and how
// many there were.
func stripU(a u128) (u128, int) {
	n := 0
	for {
		q, r := divWordU(a, 10)
		if r != 0 {
			return a, n
		}
		a = q
		n++
	}
}

// powU returns a^n and whether it fits in 128 bits, for n >= 0.
func powU(a u128, n uint) (u128, bool) {
	r := u128{Lo: 1}
	for i := bits.Len(n) - 1; i >= 0; i-- {
		var ok bool
		if r, ok = mulU(r, r); !ok {
			return r, false
		}
		if n>>uint(i)&1 != 0 {
			if r, ok = mulU(r, a); !ok {
				return r, false
			}
		}
	}
	return r, true
}

// rootU returns the integer u with u^n == a, if there is one, for n >= 1.
func rootU(a u128, n uint) (u128, bool) {
	if n == 1 || isOneU(a) {
		return a, true
	}
	if n >= 128 {
		return u128{}, false // 2^128 is too large already
	}
	// The root has at most ceil(128/n) bits; find it a bit at a time.
	var u u128
	for b := (127/int(n) + 1) - 1; b >= 0; b-- {
		t := u
		if b >= 64 {
			t.Hi |= 1 << uint(b-64)
		} else {
			t.Lo |= 1 << uint(b)
		}
		if p, ok := powU(t, n); ok && cmpU(p, a) <= 0 {
			u = t
		}
	}
	p, ok := powU(u, n)
	return u, ok && p == a
}

// strip returns x, finite and not zero, with no trailing zeros in its
// coefficient.
func strip(x dec) (c u128, e int) {
	c, n := stripU(x.c)
	return c, x.e + n
}

// The helpers below take a stripped coefficient and its exponent.

// isOne reports whether c × 10^e is 1.
func isOne(c u128, e int) bool { return isOneU(c) && e == 0 }

// belowOne reports whether c × 10^e is less than 1.
func belowOne(c u128, e int) bool { return digitsU(c)+e <= 0 }

func (x dec) isZero() bool { return x.kind == bid.KindFinite && x.c.Hi|x.c.Lo == 0 }

// integer reports whether the finite x is an integer, whether it is odd,
// and its value if that is within ±2^62.
func (x dec) integer() (isInt, odd bool, n int64, fits bool) {
	if x.isZero() {
		return true, false, 0, true
	}
	c, e := strip(x)
	if e < 0 {
		return false, false, 0, false
	}
	isInt, odd = true, e == 0 && c.Lo&1 != 0
	if e > 18 {
		return
	}
	v, ok := mulU(c, pow10U(e))
	if !ok || v.Hi != 0 || v.Lo >= 1<<62 {
		return
	}
	n = int64(v.Lo)
	if x.neg {
		n = -n
	}
	return isInt, odd, n, true
}

// Powers that are exact.

// powExact returns the value (c × 10^e)^n as N × 10^exp when that is a
// decimal of at most 38 digits; c has no trailing zeros and n is not 0.
// When ok is false and the power is not exact, it has no decimal
// expansion of 38 digits or fewer at all: either it does not terminate
// or, c^n having no trailing zeros, it has more digits than that.
func powExact(c u128, e int, n int64) (N u128, exp int64, ok bool) {
	exp = mulExp(e, n)
	switch {
	case isOneU(c):
		return c, exp, true
	case n > 127 || n < -127:
		return u128{}, 0, false // 2^128 and 1/2^128 = 5^128/10^128 are too long
	case n > 0:
		N, ok = powU(c, uint(n))
		return N, exp, ok
	}
	m := -n
	switch {
	case c.Hi == 0 && c.Lo&(c.Lo-1) == 0 || c.Lo == 0 && c.Hi&(c.Hi-1) == 0:
		// 1/2^a = 5^a / 10^a
		a := int64(lenU(c)-1) * m
		if a > 55 {
			return u128{}, 0, false
		}
		N, ok = powU(u128{Lo: 5}, uint(a))
		return N, exp - a, ok
	}
	// 1/5^b = 2^b / 10^b
	var b int64
	for {
		q, r := divWordU(c, 5)
		if r != 0 {
			break
		}
		c = q
		b++
	}
	if !isOneU(c) || b*m > 127 {
		return u128{}, 0, false
	}
	b *= m
	N = u128{Lo: 1}
	if b >= 64 {
		N = u128{Hi: 1 << uint(b-64)}
	} else {
		N.Lo <<= uint(b)
	}
	return N, exp - b, true
}

// Logarithms and the exponential in multiple precision.

// mpLn2 returns ln 2 to n words.
func mpLn2(a *arena, n int) mpf {
	if n <= len(ln2Words) {
		return mpf{m: ln2Words[len(ln2Words)-n:], exp: ln2Exp + 64*(len(ln2Words)-n)}
	}
	return mpLn2Series(a, n)
}

// mpLn2Series computes ln 2 = 2 atanh(1/3).
func mpLn2Series(a *arena, n int) mpf {
	m := a.mark()
	return a.ret(m, mpAtanh2(a, n, mpDivWord(a, n, mpWord(a, n, 1), 3)))
}

// mpLn10 returns ln 10 to n words.
func mpLn10(a *arena, n int) mpf {
	if n <= len(ln10Words) {
		return mpf{m: ln10Words[len(ln10Words)-n:], exp: ln10Exp + 64*(len(ln10Words)-n)}
	}
	return mpLn10Series(a, n)
}

// mpLn10Series computes ln 10 = 3 ln 2 + ln(5/4), where ln(5/4) is
// 2 atanh(1/9).
func mpLn10Series(a *arena, n int) mpf {
	m := a.mark()
	return a.ret(m, mpAdd(a, n, mpMulWord(a, n, mpLn2Series(a, n), 3), mpAtanh2(a, n, mpDivWord(a, n, mpWord(a, n, 1), 9))))
}

// mpAtanh2 returns 2 atanh(z) = ln((1+z)/(1-z)), for |z| <= 1/2, with a
// relative error below 2^-(64n-12).
func mpAtanh2(a *arena, n int, z mpf) mpf {
	if z.m == nil {
		return mpf{}
	}
	// atanh(z) = z × (1 + w/3 + w²/5 + ...), w = z². The terms fall by
	// at least a factor of four, so there are at most 32n of them, each
	// within a few units in the last place of the sum.
	m0 := a.mark()
	w := mpMul(a, n, z, z)
	sumSlot, pSlot := a.words(n), a.words(n)
	sum := mpNormTo(sumSlot, []uint64{1}, 0, false)
	p := mpKeep(pSlot, sum)
	for k := uint64(3); ; k += 2 {
		m := a.mark()
		nk := termWords(n, p.top()+w.top())
		p = mpKeep(pSlot, mpMul(a, nk, mpTop(p, nk), mpTop(w, nk)))
		if p.m == nil || p.top() < -(64*n+2) {
			break
		}
		sum = mpKeep(sumSlot, mpAdd(a, n, sum, mpDivWord(a, nk, p, k)))
		a.release(m)
	}
	return a.ret(m0, mpShift(mpMul(a, n, z, sum), 1))
}

// termWords returns the number of words, at most n, in which to compute
// a term below 2^top of a series whose sum is near 1 and wanted to n
// words. A term so computed, and a power it is the product of, is wrong
// by at most a few units of its own last place for each term before it;
// with the bits allowed here beyond those of the sum, that is less than
// a unit of the sum's last place for any of the at most 32n terms, as
// it is for a term computed in all n words.
func termWords(n, top int) int {
	return min(max((64*n+top+8+bits.Len(uint(n))+63)/64, 1), n)
}

// mpTop returns x truncated to its first k words, if it has more.
func mpTop(x mpf, k int) mpf {
	if d := len(x.m) - k; d > 0 {
		x.m = x.m[d:]
		x.exp += 64 * d
	}
	return x
}

// mpMulInt returns x × k.
func mpMulInt(a *arena, n int, x mpf, k int64) mpf {
	if k < 0 {
		return mpNeg(mpMulWord(a, n, x, uint64(-k)))
	}
	return mpMulWord(a, n, x, uint64(k))
}

// mpDec returns (-1)^neg × c × 10^e to within four units in the last of n
// words.
func mpDec(a *arena, n int, neg bool, c u128, e int) mpf {
	m := a.mark()
	v := mpInt(a, n, c.Hi, c.Lo)
	switch {
	case e > 0 && e < len(pow10Word):
		v = mpMulWord(a, n, v, pow10Word[e])
	case e < 0 && -e < len(pow10Word):
		v = mpDivWord(a, n, v, pow10Word[-e])
	case e > 0 && e <= maxPow10Exact:
		v = mpMul(a, n, v, mpPow10Exact(a, e))
	case e < 0 && -e <= maxPow10Exact:
		v = mpDiv(a, n, v, mpPow10Exact(a, -e))
	case e != 0:
		v = mpMul(a, n, v, mpPow10(a, n, e))
	}
	v.neg = neg
	return a.ret(m, v)
}

// maxPow10Exact is the largest k for which mpPow10Exact gives 10^k: the
// square of the largest power of ten of 128 bits.
const maxPow10Exact = 76

// mpPow10Exact returns 10^k, exactly, for k in [0, maxPow10Exact]. A
// product or a quotient by it is one operation, where a power of ten
// from mpPow10 costs a chain of squarings and its error.
func mpPow10Exact(a *arena, k int) mpf {
	var w [4]uint64
	if k <= 38 {
		p := pow10U(k)
		w[0], w[1] = p.Lo, p.Hi
	} else {
		// 10^38 × 10^(k-38), two words by two.
		x, y := pow10U(38), pow10U(k-38)
		var c uint64
		w[1], w[0] = bits.Mul64(x.Lo, y.Lo)
		hi, lo := bits.Mul64(x.Hi, y.Lo)
		w[1], c = bits.Add64(w[1], lo, 0)
		w[2] = hi + c
		hi, lo = bits.Mul64(x.Lo, y.Hi)
		w[1], c = bits.Add64(w[1], lo, 0)
		w[2], c = bits.Add64(w[2], hi, c)
		w[3] = c
		hi, lo = bits.Mul64(x.Hi, y.Hi)
		w[2], c = bits.Add64(w[2], lo, 0)
		w[3] += hi + c
	}
	l := len(w)
	for w[l-1] == 0 {
		l--
	}
	return mpNormTo(a.words(l), w[:l], 0, false)
}

// mpScale10 returns v × 10^-q to within four units in the last of n
// words.
func mpScale10(a *arena, n int, v mpf, q int) mpf {
	m := a.mark()
	switch {
	case q == 0:
		return mpNorm(a, n, v.m, v.exp, v.neg)
	case q < 0 && q >= -maxPow10Exact:
		return a.ret(m, mpMul(a, n, v, mpPow10Exact(a, -q)))
	case q > 0 && q <= maxPow10Exact:
		return a.ret(m, mpDiv(a, n, v, mpPow10Exact(a, q)))
	}
	return a.ret(m, mpMul(a, n, v, mpPow10(a, n, -q)))
}

// mpPowInt returns b^k for a positive b and a k of at most 2^20 in
// magnitude, by repeated squaring, with a relative error below
// 2^-(64n-25) if b is within four units in its last place: each product
// adds a unit and the error of b, and each squaring doubles what there
// is, which comes to under 8|k| units.
func mpPowInt(a *arena, n int, b mpf, k int64) (mpf, int) {
	u := uint64(k)
	if k < 0 {
		u = -u
	}
	// b is between 2^(top-1) and 2^top. A power beyond 2^±(2^24) is
	// beyond every format, and is not computed: its exponent need not
	// fit an int.
	huge, tiny := false, false
	if t := int64(b.top()); t > 1 {
		huge = u*uint64(t-1) > 1<<24
	} else if t < 0 {
		tiny = u*uint64(-t) > 1<<24
	}
	switch {
	case huge && k > 0, tiny && k < 0:
		return mpf{}, evOverflow
	case huge || tiny:
		return mpf{}, evUnderflow
	}
	m0 := a.mark()
	slot := a.words(n)
	r := mpNormTo(slot, []uint64{1}, 0, false)
	for i := bits.Len64(u) - 1; i >= 0; i-- {
		m := a.mark()
		t := mpMul(a, n, r, r)
		if u>>uint(i)&1 != 0 {
			t = mpMul(a, n, t, b)
		}
		r = mpKeep(slot, t)
		a.release(m)
	}
	if k < 0 {
		r = mpDiv(a, n, mpWord(a, n, 1), r)
	}
	return a.ret(m0, r), evValue
}

// maxPowInt is the largest count mpPowInt takes; beyond it a power is
// the exponential of a logarithm.
const maxPowInt = 1 << 20

// mpLn returns ln(c × 10^e), for 0 < c < 10^38, with a relative error
// below 2^-(64n-14). The value must not be 1.
func mpLn(a *arena, n int, c u128, e int) mpf {
	// Write the value as m × 2^b × 10^(e+d), with c/10^d in [0.3, 3)
	// and m in [0.707, 1.415]. Then
	//
	//	ln = 2 atanh((m-1)/(m+1)) + b ln 2 + (e+d) ln 10
	//
	// and (m-1)/(m+1) is the quotient of two integers that are exact:
	// (c - 2^b × 10^d) and (c + 2^b × 10^d), with the power of two on
	// the other side when b is negative. A value near 1 has b = 0 and
	// e+d = 0 on either side of 1, so nothing cancels there; elsewhere
	// the sum is at least a third of its largest term.
	d := digitsU(c) - 1
	if p := pow10U(d); cmpU(c, addU(p, addU(p, p))) >= 0 {
		d++
	}
	p := pow10U(d)
	b := lenU(c) - lenU(p)
	// tc/tp is c/p scaled into (1/2, 2), from the top 64 bits of each.
	m0 := a.mark()
	tc := mpInt(a, 1, c.Hi, c.Lo).m[0]
	tp := mpInt(a, 1, p.Hi, p.Lo).m[0]
	if cmpU(mulWordU(tc, 70), mulWordU(tp, 99)) >= 0 {
		b++ // c/(p × 2^b) was at least 99/70, just over the square root of 2
	} else if cmpU(mulWordU(tc, 99), mulWordU(tp, 70)) < 0 {
		b-- // or below 70/99
	}
	num, den := mpInt(a, n, c.Hi, c.Lo), mpInt(a, n, p.Hi, p.Lo)
	if b < 0 {
		num = mpShift(num, -b)
	} else {
		den = mpShift(den, b)
	}
	z := mpDiv(a, n, mpSub(a, n, num, den), mpAdd(a, n, num, den))
	v := mpAtanh2(a, n, z)
	if b != 0 {
		v = mpAdd(a, n, v, mpMulInt(a, n, mpLn2(a, n), int64(b)))
	}
	if e+d != 0 {
		v = mpAdd(a, n, v, mpMulInt(a, n, mpLn10(a, n), int64(e+d)))
	}
	return a.ret(m0, v)
}

func mulWordU(a, b uint64) u128 {
	hi, lo := bits.Mul64(a, b)
	return u128{Hi: hi, Lo: lo}
}

// onePlus returns 1 + x as c × 10^e with c below 10^38 and no trailing
// zeros, if it can be written so. x is finite, not zero and above -1.
func onePlus(x dec) (c u128, e int, ok bool) {
	c, e = strip(x)
	switch {
	case e >= 0:
		if x.neg || e > 37 {
			return c, e, false // x is at least 1, so not negative here
		}
		v, fits := mulU(c, pow10U(e))
		if !fits || cmpU(v, pow10U(38)) >= 0 {
			return c, e, false
		}
		v = addU(v, u128{Lo: 1})
		v, n := stripU(v)
		return v, n, cmpU(v, pow10U(38)) < 0
	case -e > 37:
		return c, e, false
	}
	one := pow10U(-e)
	if x.neg {
		c = subU(one, c) // x > -1, so c < 10^-e
	} else {
		c = addU(one, c) // below 10^38: c has at most 34 digits
	}
	c, n := stripU(c)
	return c, e + n, true
}

// mpLn1p returns ln(1+x) for a finite x above -1 and not zero, with a
// relative error below 2^-(64n-14).
func mpLn1p(a *arena, n int, x dec) mpf {
	if c, e, ok := onePlus(x); ok {
		return mpLn(a, n, c, e)
	}
	m := a.mark()
	c, e := strip(x)
	xf := mpDec(a, n, x.neg, c, e)
	two := mpWord(a, n, 2)
	if e < 0 {
		// |x| is below 10^-4: ln(1+x) = 2 atanh(x/(2+x)).
		return a.ret(m, mpAtanh2(a, n, mpDiv(a, n, xf, mpAdd(a, n, two, xf))))
	}
	// x is 10^37 or more: ln(1+x) = ln x + ln(1 + 1/x).
	inv := mpDiv(a, n, mpWord(a, n, 1), xf)
	return a.ret(m, mpAdd(a, n, mpLn(a, n, c, e), mpAtanh2(a, n, mpDiv(a, n, inv, mpAdd(a, n, two, inv)))))
}

// Outcomes of an evaluation other than a value: the result is a known
// exact number (1, unless the caller names another) plus or minus less
// than the working precision can show, or beyond the range of every
// format.
const (
	evValue = iota
	evAbove
	evBelow
	evOverflow
	evUnderflow
)

// mpExp returns e^t with a relative error below 2^-(64n-30), provided t
// is itself within 2^-(64n-14) of the exact argument, in relative terms.
// t must not be zero.
func mpExp(a *arena, n int, t mpf) (mpf, int) {
	switch {
	case t.top() > 15:
		// |t| is 2^15 or more, and e^±32768 is beyond decimal128.
		if t.neg {
			return mpf{}, evUnderflow
		}
		return mpf{}, evOverflow
	case t.top() < -(64*n - 50):
		// |e^t - 1| < 2|t| is below what n words can separate from 1,
		// and below one unit of any decimal precision this is used for.
		if t.neg {
			return mpf{}, evBelow
		}
		return mpf{}, evAbove
	}
	// e^t = 2^k × e^r, with r = t - k ln 2 in about [-0.35, 0.35]. The
	// subtraction is done with a word to spare, so that what it cancels
	// costs nothing.
	if t.top() < -1 {
		return mpExpSmall(a, n, t), evValue // |t| < 1/4
	}
	m := a.mark()
	ln2 := mpLn2(a, n+1)
	half := mpf{m: []uint64{1 << 63}, exp: -64, neg: t.neg}
	_, lo, _, _ := mpFloor(mpAdd(a, 2, mpDiv(a, 2, t, ln2), half), 0)
	k := int64(lo) // |t/ln 2| + 1/2 truncated: the nearest integer
	if t.neg {
		k = -k
	}
	r := mpSub(a, n+1, t, mpMulInt(a, n+1, ln2, k))
	return a.ret(m, mpShift(mpExpSmall(a, n, mpNorm(a, n, r.m, r.exp, r.neg)), int(k))), evValue
}

// mpExpSmall returns e^r for |r| below 1/2.
func mpExpSmall(a *arena, n int, r mpf) mpf {
	// e^r = (e^(r/2^j))^(2^j). The series for the inner exponential
	// needs about 64n/(j+1) terms, each within a unit of the sum, and
	// each squaring doubles the relative error and adds a unit: about
	// 2^(j+6) units in the last place at the end, 2^18 for j = 12.
	const j = 12
	m0 := a.mark()
	sumSlot, termSlot := a.words(n), a.words(n)
	sum := mpNormTo(sumSlot, []uint64{1}, 0, false)
	if r.m != nil {
		r = mpShift(r, -j)
		m := a.mark()
		sum = mpKeep(sumSlot, mpAdd(a, n, sum, r))
		a.release(m)
		term := r
		for i := uint64(2); ; i++ {
			m := a.mark()
			ni := termWords(n, term.top()+r.top())
			term = mpKeep(termSlot, mpDivWord(a, ni, mpMul(a, ni, mpTop(term, ni), mpTop(r, ni)), i))
			if term.m == nil || term.top() < -(64*n+2) {
				break
			}
			sum = mpKeep(sumSlot, mpAdd(a, n, sum, term))
			a.release(m)
		}
	}
	for i := 0; i < j; i++ {
		m := a.mark()
		sum = mpKeep(sumSlot, mpMul(a, n, sum, sum))
		a.release(m)
	}
	return a.ret(m0, sum)
}

// guardBits is the number of low bits of an n-word value that the
// evaluations here may have wrong: their relative errors are below
// 2^-(64n-30), and scaling by a power of ten adds a few units.
const guardBits = 44

// mpDigits returns the first k digits of v, which is positive: the
// integer c with 10^(k-1) <= c < 10^k and c <= v/10^q < c+1. ok is
// false if an error of up to 2^guardBits units in the last place of v
// could change c, or make v/10^q an integer.
func mpDigits(a *arena, n int, v mpf, k int) (c u128, q int, ok bool) {
	// The leading digit's exponent is floor(log10 v), which is
	// floor(log2 v) × log10(2) to within one; 78913/2^18 is log10(2)
	// to eight places.
	q = int((int64(v.top()-1)*78913)>>18) - (k - 1)
	for try := 0; try < 4; try++ {
		m := a.mark()
		hi, lo, near, fits := mpFloor(mpScale10(a, n, v, q), guardBits)
		a.release(m)
		c = u128{Hi: hi, Lo: lo}
		switch {
		case !fits || cmpU(c, pow10U(k)) >= 0:
			q++
		case cmpU(c, pow10U(k-1)) < 0:
			q--
		default:
			return c, q, !near
		}
	}
	return c, q, false
}

// An eval is a value for ziv to compute: one of the functions below of
// the operands it holds.
type eval struct {
	op    evalOp
	c     u128 // the operand, or the base of a power: c × 10^e, c stripped
	e     int
	neg   bool  // the sign of the argument of evalExp, or of evalPow's y
	n     int64 // the count of a power or a root
	x     dec   // the operand of a compound
	c2    u128  // evalPow's y: c2 × 10^e2
	e2    int
	bit   int  // evalCompoundFar: 2|n|/x is below 2^bit
	exact bool // evalCompoundFar: x^n is a short decimal
}

type evalOp uint8

const (
	evalSqrt        evalOp = iota // the square root of c
	evalLog                       // |ln(c × 10^e)|
	evalLog10                     // |log10(c × 10^e)|
	evalPowInt                    // (c × 10^e)^n by repeated squaring
	evalCompoundFar               // (1+x)^n for an x of 10^37 or more
	evalExp                       // e^(±c × 10^e)
	evalPowLn                     // (c × 10^e)^n as e^(n ln)
	evalCompound                  // (1+x)^n as e^(n ln(1+x))
	evalRootn                     // the nth root of c × 10^e
	evalPow                       // (c × 10^e)^(±c2 × 10^e2)
)

// run computes the value in n words, with a relative error below
// 2^-(64n-30), or reports one of the outcomes that are not a value.
func (v *eval) run(a *arena, n int) (mpf, int) {
	var t mpf // the argument of an exponential
	switch v.op {
	case evalSqrt:
		return mpSqrt(a, n, mpInt(a, n, v.c.Hi, v.c.Lo)), evValue
	case evalLog, evalLog10:
		r := mpLn(a, n, v.c, v.e)
		if v.op == evalLog10 {
			r = mpDiv(a, n, r, mpLn10(a, n))
		}
		r.neg = false
		return r, evValue
	case evalPowInt:
		return mpPowInt(a, n, mpDec(a, n, false, v.c, v.e), v.n)
	case evalCompoundFar:
		// (1+x)^n is x^n × (1+1/x)^n, whose second factor is within
		// 2|n|/x of 1. A precision that cannot show that difference
		// would have to grow until it could, which for the largest x is
		// thousands of digits; instead, at such a precision the result
		// is x^n and a bit, more or less of it by the sign of n. If x^n
		// is not a short decimal, the bit is far less than the error
		// already allowed for in its digits.
		switch {
		case v.bit >= -(64*n - 50):
			t = mpMulInt(a, n, mpLn1p(a, n, v.x), v.n)
		case !v.exact:
			t = mpMulInt(a, n, mpLn(a, n, v.c, v.e), v.n)
		case v.n < 0:
			return mpf{}, evBelow
		default:
			return mpf{}, evAbove
		}
	case evalExp:
		t = mpDec(a, n, v.neg, v.c, v.e)
	case evalPowLn:
		t = mpMulInt(a, n, mpLn(a, n, v.c, v.e), v.n)
	case evalCompound:
		t = mpMulInt(a, n, mpLn1p(a, n, v.x), v.n)
	case evalRootn:
		u := uint64(v.n)
		if v.n < 0 {
			u = -u
		}
		t = mpDivWord(a, n, mpLn(a, n, v.c, v.e), u)
		t.neg = t.neg != (v.n < 0)
	case evalPow:
		t = mpMul(a, n, mpDec(a, n, v.neg, v.c2, v.e2), mpLn(a, n, v.c, v.e))
	}
	return mpExp(a, n, t)
}

// zivStore is the number of words of an evaluation's arena. The first
// evaluation of every function fits in it, at either width; see
// TestArenaStore.
const zivStore = 256

// ziv returns the result for the positive, inexact value that v
// computes, times 10^scale and with the sign neg.
//
// v.run computes the value in n words with a relative error below
// 2^-(64n-30). The first p+3 digits of that are taken, and they are the
// digits of the exact value unless the approximation is so close to a
// number of p+3 digits that its error could carry it across; then the
// precision is doubled and the value computed again (Ziv's strategy).
// The loop ends because the exact value is not such a number: every
// caller has returned its exact results already, and what is left is
// irrational, or has more digits than p+3. The digits and the fact that
// more follow are what the rounding routine needs to round correctly
// in every case, a subnormal result included.
//
// Some values are a known number and a difference too small for any
// reasonable precision to show: e^t for a tiny t is 1 and a bit. The
// evaluation reports those as evAbove or evBelow, and the result is then the number
// just above or below near, which is 1 unless the caller gives another.
func (f *format) ziv(neg bool, scale int64, near dec, v *eval) result {
	k := f.p + 3
	if near.c.Hi|near.c.Lo == 0 {
		near.c.Lo = 1
	}
	var store [zivStore]uint64
	a := arena{buf: store[:0]}
	for n := f.words; ; n *= 2 {
		if n > 1<<12 {
			// A quarter of a million bits have not separated the value
			// from a number of p+3 digits: it is one, and a caller has
			// failed to return an exact result.
			panic("decimal: exact result reached the rounding loop")
		}
		a.release(0)
		x, ev := v.run(&a, n)
		r := result{dec: dec{neg: neg}, sticky: true}
		switch ev {
		case evAbove, evBelow:
			// near written with at least p+3 digits, and for a value
			// just below it one unit less: the digits of the value,
			// with more to follow either way.
			r.c, r.e = near.c, near.e
			if j := k - digitsU(r.c); j > 0 {
				r.c, _ = mulU(r.c, pow10U(j))
				r.e -= j
			}
			if ev == evBelow {
				r.c = subU(r.c, u128{Lo: 1})
			}
		case evOverflow:
			r.c, r.e = u128{Lo: 1}, farExp
		case evUnderflow:
			r.c, r.e = u128{Lo: 1}, -farExp
		default:
			c, q, ok := mpDigits(&a, n, x, k)
			if !ok {
				continue
			}
			r.c, r.e = c, q
		}
		r.e = clampExp(int64(r.e) + scale)
		return r
	}
}

// The functions. Each takes operands that are not NaNs, except where it
// says so.

// sqrtDec returns the square root of x.
func (f *format) sqrtDec(x dec) result {
	switch {
	case x.isZero():
		return resZero(x.neg, floorDiv(int64(x.e), 2)) // the root of -0 is -0
	case x.neg:
		return resNaN()
	case x.kind == bid.KindInf:
		return resInf(false)
	}
	c, e := strip(x)
	if e&1 != 0 {
		c, _ = mulU(c, u128{Lo: 10})
		e--
	}
	if r, ok := rootU(c, 2); ok {
		return f.exact(false, r, int64(e/2), floorDiv(int64(x.e), 2))
	}
	return f.ziv(false, int64(e/2), dec{}, &eval{op: evalSqrt, c: c})
}

// floorDiv returns the floor of a/b.
func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// expDec returns e^x.
func (f *format) expDec(x dec) result {
	switch {
	case x.kind == bid.KindInf:
		if x.neg {
			return resZero(false, 0)
		}
		return resInf(false)
	case x.isZero():
		return resOne()
	}
	c, e := strip(x)
	return f.ziv(false, 0, dec{}, &eval{op: evalExp, c: c, e: e, neg: x.neg})
}

// logDec returns the natural logarithm of x, or the one to base 10.
func (f *format) logDec(x dec, base10 bool) result {
	switch {
	case x.isZero():
		return resInf(true)
	case x.neg:
		return resNaN()
	case x.kind == bid.KindInf:
		return resInf(false)
	}
	c, e := strip(x)
	if isOneU(c) && (base10 || e == 0) {
		// The logarithm of 1 is 0, and log10 of 10^e is e. No other
		// logarithm is rational: c^q = 10^p needs c to be a power of
		// ten, and e^(p/q) is never rational for p not zero.
		a := int64(e)
		if a < 0 {
			a = -a
		}
		return f.exact(e < 0, u128{Lo: uint64(a)}, 0, 0)
	}
	op := evalLog
	if base10 {
		op = evalLog10
	}
	return f.ziv(belowOne(c, e), 0, dec{}, &eval{op: op, c: c, e: e})
}

// pownDec returns x^n. x may be a NaN if n is 0.
func (f *format) pownDec(x dec, n int64) result {
	neg := x.neg && n&1 != 0
	switch {
	case n == 0:
		return resOne()
	case x.isZero():
		if n < 0 {
			return resInf(neg)
		}
		return resZero(neg, mulExp(x.e, n))
	case x.kind == bid.KindInf:
		if n < 0 {
			return resZero(neg, 0)
		}
		return resInf(neg)
	}
	c, e := strip(x)
	if N, exp, ok := powExact(c, e, n); ok {
		return f.exact(neg, N, exp, mulExp(x.e, n))
	}
	op := evalPowLn
	if n <= maxPowInt && n >= -maxPowInt {
		op = evalPowInt
	}
	return f.ziv(neg, 0, dec{}, &eval{op: op, c: c, e: e, n: n})
}

// compoundDec returns (1+x)^n. x may be a NaN if n is 0.
func (f *format) compoundDec(x dec, n int64) result {
	switch {
	case x.kind == bid.KindNaN:
		return resOne() // n is 0
	case x.kind == bid.KindInf:
		switch {
		case x.neg:
			return resNaN()
		case n == 0:
			return resOne()
		case n < 0:
			return resZero(false, 0)
		}
		return resInf(false)
	case x.isZero() || n == 0:
		if x.neg && !x.isZero() {
			if c, e := strip(x); !belowOne(c, e) && !isOne(c, e) {
				return resNaN() // x < -1, even for n = 0
			}
		}
		return resOne()
	}
	// x is finite and not zero, and n is not zero.
	c, e := strip(x)
	if x.neg && !belowOne(c, e) {
		if !isOne(c, e) {
			return resNaN() // x < -1
		}
		if n < 0 {
			return resInf(false) // 0^n, a division by zero
		}
		return resZero(false, 0)
	}
	b, eb, short := onePlus(x)
	if short {
		if N, exp, ok := powExact(b, eb, n); ok {
			return f.exact(false, N, exp, mulExp(min(x.e, 0), n))
		}
	}
	if short && n <= maxPowInt && n >= -maxPowInt {
		// 1+x is short enough to be held exactly: raise it.
		return f.ziv(false, 0, dec{}, &eval{op: evalPowInt, c: b, e: eb, n: n})
	}
	if short || e < 0 {
		return f.ziv(false, 0, dec{}, &eval{op: evalCompound, x: x, n: n})
	}
	// x is 10^37 or more: the result is x^n, a little more or less of
	// it; see evalCompoundFar.
	N, exp, exact := powExact(c, e, n)
	a := uint64(n)
	if n < 0 {
		a = -a
	}
	bit := bits.Len64(a) + 1 - 3*(digitsU(c)-1+e) // 2|n|/x < 2^bit: 10 > 2^3
	return f.ziv(false, 0, dec{c: N, e: clampExp(exp)},
		&eval{op: evalCompoundFar, c: c, e: e, n: n, x: x, bit: bit, exact: exact})
}

// rootnDec returns the nth root of x.
func (f *format) rootnDec(x dec, n int64) result {
	neg := x.neg && n&1 != 0
	switch {
	case n == 0:
		return resNaN()
	case x.isZero():
		if n < 0 {
			return resInf(neg)
		}
		return resZero(neg, floorDiv(int64(x.e), n))
	case x.neg && n&1 == 0:
		return resNaN()
	case x.kind == bid.KindInf:
		if n < 0 {
			return resZero(neg, 0)
		}
		return resInf(neg)
	}
	c, e := strip(x)
	a := uint64(n)
	if n < 0 {
		a = -a
	}
	if e == 0 || a <= farExp && int64(e)%int64(a) == 0 {
		// The root is rational only if c is an nth power and the
		// exponent a multiple of n: c has no trailing zeros, so
		// neither has an nth power it might be.
		if u, ok := rootU(c, uint(min(a, 128))); ok {
			sgn, q := int64(1), 0
			if n < 0 {
				sgn = -1
			}
			if e != 0 {
				q = e / int(a)
			}
			if N, exp, ok := powExact(u, q, sgn); ok {
				return f.exact(neg, N, exp, floorDiv(int64(x.e), n))
			}
		}
	}
	return f.ziv(neg, 0, dec{}, &eval{op: evalRootn, c: c, e: e, n: n})
}

// powDec returns x^y. Either operand may be a NaN.
func (f *format) powDec(x, y dec) result {
	xNaN, yNaN := x.kind == bid.KindNaN, y.kind == bid.KindNaN
	var cx u128
	var ex int
	xOne := false
	if x.kind == bid.KindFinite && !x.isZero() {
		cx, ex = strip(x)
		xOne = isOne(cx, ex)
	}
	switch {
	case y.isZero():
		return resOne() // even for a NaN x
	case xOne && !x.neg && y.kind != bid.KindFinite:
		return resOne() // even for a NaN y
	case xNaN || yNaN:
		return resNaN()
	}
	var yInt, yOdd, yFits bool
	var yn int64
	if y.kind == bid.KindFinite {
		yInt, yOdd, yn, yFits = y.integer()
	}
	neg := x.neg && yOdd
	switch {
	case x.isZero():
		if y.neg {
			return resInf(neg)
		}
		return resZero(neg, 0)
	case y.kind == bid.KindInf:
		switch {
		case xOne: // x is -1
			return resOne()
		case (x.kind == bid.KindFinite && belowOne(cx, ex)) == y.neg:
			return resInf(false)
		}
		return resZero(false, 0)
	case x.kind == bid.KindInf:
		if y.neg {
			return resZero(neg, 0)
		}
		return resInf(neg)
	case yFits:
		return f.pownDec(x, yn)
	case x.neg && !yInt:
		return resNaN()
	}
	// x is finite and not zero, positive unless y is an integer, and y
	// is finite, not zero, and a fraction or an integer beyond ±2^62.
	cy, ey := strip(y)
	pref := prefPow(y, x.e)
	switch {
	case isOneU(cx):
		// x is 10^ex, and x^y is 10^(ex × y): exact if that exponent
		// is an integer, and irrational if not.
		a := int64(ex)
		if a < 0 {
			a = -a
		}
		if a == 0 {
			return f.exact(neg, u128{Lo: 1}, 0, pref) // |x| is 1
		}
		m, ok := mulU(cy, u128{Lo: uint64(a)})
		if ey > 0 {
			m, ok = u128{Lo: farExp}, true
		} else if ey < 0 {
			if ok = ok && -ey <= 38; ok {
				m, ok = divPow10U(m, -ey)
			}
		}
		if ok {
			exp := int64(farExp)
			if m.Hi == 0 && m.Lo < farExp {
				exp = int64(m.Lo)
			}
			if (ex < 0) != y.neg {
				exp = -exp
			}
			return f.exact(neg, u128{Lo: 1}, exp, pref)
		}
	case !yInt:
		// y is p/q in lowest terms with q > 1 a divisor of 10^-ey, and
		// x^y is rational only if x is the qth power of a rational:
		// cx = u^q and q divides ex. Then x^y = (u × 10^(ex/q))^p.
		v2, v5 := 0, 0
		for t := cy; ; v5++ {
			q, r := divWordU(t, 5)
			if r != 0 {
				break
			}
			t = q
		}
		if cy.Lo != 0 {
			v2 = bits.TrailingZeros64(cy.Lo)
		} else {
			v2 = 64 + bits.TrailingZeros64(cy.Hi)
		}
		a, b := max(-ey-v2, 0), max(-ey-v5, 0)
		if a > 6 || b > 2 {
			break // q would be more than 127, and cx is below 2^128
		}
		q := int64(1) << uint(a)
		for i := 0; i < b; i++ {
			q *= 5
		}
		if q > 127 || int64(ex)%q != 0 {
			break
		}
		u, ok := rootU(cx, uint(q))
		if !ok {
			break
		}
		// p = cy / (2^(-ey-a) × 5^(-ey-b)), an exact division.
		p := cy
		if s := uint(-ey - a); s >= 64 {
			p = u128{Lo: p.Hi >> (s - 64)}
		} else if s > 0 {
			p = u128{Hi: p.Hi >> s, Lo: p.Lo>>s | p.Hi<<(64-s)}
		}
		for i := -ey - b; i > 0; i-- {
			p, _ = divWordU(p, 5)
		}
		if p.Hi != 0 || p.Lo > 1<<31 {
			break // u is 2 or more: the power is far out of range
		}
		pn := int64(p.Lo)
		if y.neg {
			pn = -pn
		}
		if N, exp, ok := powExact(u, int(int64(ex)/q), pn); ok {
			return f.exact(false, N, exp, pref)
		}
	}
	return f.ziv(neg, 0, dec{}, &eval{op: evalPow, c: cx, e: ex, neg: y.neg, c2: cy, e2: ey})
}

// prefPow returns floor(y × q), the preferred exponent of an exact x^y
// where q is the exponent of x, limited to ±farExp.
func prefPow(y dec, q int) int64 {
	a := int64(q)
	neg := y.neg != (a < 0)
	if a < 0 {
		a = -a
	}
	m, ok := mulU(y.c, u128{Lo: uint64(a)})
	exact := true
	switch {
	case ok && m.Hi|m.Lo == 0:
	case !ok || y.e > 0 && m.Hi|m.Lo != 0 && (y.e > 7 || m.Hi != 0 || m.Lo >= farExp):
		m = u128{Lo: farExp}
	case y.e > 0:
		m, _ = mulU(m, pow10U(y.e))
	case y.e < -38:
		exact, m = m.Hi|m.Lo == 0, u128{}
	case y.e < 0:
		m, exact = divPow10U(m, -y.e)
	}
	v := int64(farExp)
	if m.Hi == 0 && m.Lo < farExp {
		v = int64(m.Lo)
	}
	if neg {
		v = -v
		if !exact {
			v--
		}
	}
	return v
}

// scaleBDec returns x × 10^n.
func (f *format) scaleBDec(x dec, n int64) result {
	if x.kind != bid.KindFinite {
		return result{dec: x}
	}
	n = min(max(n, -farExp), farExp)
	return result{dec: dec{neg: x.neg, c: x.c, e: clampExp(int64(x.e) + n)}}
}

// logBDec returns the exponent of x as a decimal: floor(log10 |x|).
func (f *format) logBDec(x dec) result {
	switch {
	case x.kind == bid.KindInf:
		return resInf(false)
	case x.isZero():
		return resInf(true)
	}
	l := digitsU(x.c) - 1 + x.e
	neg := l < 0
	if neg {
		l = -l
	}
	return result{dec: dec{neg: neg, c: u128{Lo: uint64(l)}}}
}

// nextUpDec returns the least value of the format above x.
func (f *format) nextUpDec(x dec) result {
	largest := result{dec: dec{c: subU(pow10U(f.p), u128{Lo: 1}), e: f.maxExp}}
	switch {
	case x.kind == bid.KindInf:
		if x.neg {
			largest.neg = true
			return largest
		}
		return resInf(false)
	case x.isZero():
		return result{dec: dec{c: u128{Lo: 1}, e: f.minExp}}
	}
	// Write x with as many digits as the format and its least exponent
	// allow; its neighbours are then one unit away.
	c, e := x.c, x.e
	if k := min(f.p-digitsU(c), e-f.minExp); k > 0 {
		c, _ = mulU(c, pow10U(k))
		e -= k
	}
	one := u128{Lo: 1}
	if !x.neg {
		if c = addU(c, one); c == pow10U(f.p) {
			if c, e = pow10U(f.p-1), e+1; e > f.maxExp {
				return resInf(false)
			}
		}
	} else if c = subU(c, one); e > f.minExp && cmpU(c, pow10U(f.p-1)) < 0 {
		c, e = subU(pow10U(f.p), one), e-1
	}
	return result{dec: dec{neg: x.neg, c: c, e: e}}
}

// nextDownDec returns the greatest value of the format below x.
func (f *format) nextDownDec(x dec) result {
	x.neg = !x.neg
	r := f.nextUpDec(x)
	r.neg = !r.neg
	return r
}
