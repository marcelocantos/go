// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal

import "math/bits"

// This file is a small multiple-precision binary floating-point
// arithmetic, enough to evaluate the elementary functions of
// elementary.go to any precision. It exists so that those functions can
// be correctly rounded without importing math/big, which this package
// cannot (strconv imports it) and math/decimal should not (math/big
// brings in fmt).
//
// Every operation takes its precision n, in 64-bit words, and truncates
// its result toward zero to n words. A result is therefore within one
// unit in the last place of the exact result of the operation on the
// operands it was given: a relative error below 2^-(64n-1). The
// functions built from them state their own bounds.

// An mpf is the number (-1)^neg × m × 2^exp, where m is the integer whose
// words, least significant first, are the slice. A nonzero mpf returned
// by a function of this file is normalized: the top bit of its last word
// is set. A nil slice is zero.
type mpf struct {
	m   []uint64
	exp int
	neg bool
}

// top returns the exponent of the power of two just above |x|:
// 2^(top-1) <= |x| < 2^top for a normalized x.
func (x mpf) top() int { return x.exp + 64*len(x.m) }

// mpNorm returns (-1)^neg × w × 2^exp truncated to n words, where w is an
// integer of any length, least significant word first.
func mpNorm(n int, w []uint64, exp int, neg bool) mpf {
	l := len(w)
	for l > 0 && w[l-1] == 0 {
		l--
	}
	if l == 0 {
		return mpf{}
	}
	lz := uint(bits.LeadingZeros64(w[l-1]))
	m := make([]uint64, n)
	for i := n - 1; i >= 0; i-- {
		j := l - n + i
		var hi, lo uint64
		if j >= 0 {
			hi = w[j]
		}
		if j >= 1 {
			lo = w[j-1]
		}
		m[i] = hi << lz
		if lz != 0 {
			m[i] |= lo >> (64 - lz)
		}
	}
	return mpf{m, exp + 64*(l-n) - int(lz), neg}
}

// mpWord returns the integer w as an mpf of n words.
func mpWord(n int, w uint64) mpf { return mpNorm(n, []uint64{w}, 0, false) }

// mpInt returns the integer hi×2^64 + lo as an mpf of n words; it is
// exact for n >= 2.
func mpInt(n int, hi, lo uint64) mpf { return mpNorm(n, []uint64{lo, hi}, 0, false) }

// mpNeg returns -x.
func mpNeg(x mpf) mpf {
	x.neg = !x.neg
	return x
}

// mpShift returns x × 2^k.
func mpShift(x mpf, k int) mpf {
	if x.m != nil {
		x.exp += k
	}
	return x
}

// mpMul returns x × y truncated to n words.
func mpMul(n int, x, y mpf) mpf {
	if x.m == nil || y.m == nil {
		return mpf{}
	}
	w := make([]uint64, len(x.m)+len(y.m))
	for i, a := range x.m {
		var c uint64
		for j, b := range y.m {
			hi, lo := bits.Mul64(a, b)
			var c1, c2 uint64
			lo, c1 = bits.Add64(lo, w[i+j], 0)
			lo, c2 = bits.Add64(lo, c, 0)
			w[i+j] = lo
			c = hi + c1 + c2 // cannot overflow: a×b + two words < 2^128
		}
		w[i+len(y.m)] = c
	}
	return mpNorm(n, w, x.exp+y.exp, x.neg != y.neg)
}

// mpMulWord returns x × w truncated to n words.
func mpMulWord(n int, x mpf, w uint64) mpf {
	if x.m == nil || w == 0 {
		return mpf{}
	}
	p := make([]uint64, len(x.m)+1)
	var c uint64
	for i, a := range x.m {
		hi, lo := bits.Mul64(a, w)
		var c1 uint64
		p[i], c1 = bits.Add64(lo, c, 0)
		c = hi + c1
	}
	p[len(x.m)] = c
	return mpNorm(n, p, x.exp, x.neg)
}

// mpDivWord returns x / w truncated to n words. w must not be zero.
func mpDivWord(n int, x mpf, w uint64) mpf {
	if x.m == nil {
		return mpf{}
	}
	// Divide x.m scaled by whole words, so that the quotient has a word
	// more than n whatever w is.
	k := max(n-len(x.m), 0) + 1
	q := make([]uint64, len(x.m)+k)
	var r uint64
	for i := len(x.m) - 1; i >= 0; i-- {
		q[i+k], r = bits.Div64(r, x.m[i], w)
	}
	for i := k - 1; i >= 0; i-- {
		q[i], r = bits.Div64(r, 0, w)
	}
	return mpNorm(n, q, x.exp-64*k, x.neg)
}

// shlWords returns w × 2^s as a new slice.
func shlWords(w []uint64, s uint) []uint64 {
	ws, bs := int(s/64), s%64
	r := make([]uint64, len(w)+ws+1)
	for i, x := range w {
		r[i+ws] |= x << bs
		if bs != 0 {
			r[i+ws+1] = x >> (64 - bs)
		}
	}
	return r
}

// cmpWords compares the integers a and b, which have the same length.
func cmpWords(a, b []uint64) int {
	for i := len(a) - 1; i >= 0; i-- {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// mpAdd returns x + y truncated to n words.
func mpAdd(n int, x, y mpf) mpf {
	if x.m == nil {
		return mpNorm(n, y.m, y.exp, y.neg)
	}
	if y.m == nil {
		return mpNorm(n, x.m, x.exp, x.neg)
	}
	if x.top() < y.top() {
		x, y = y, x
	}
	// If y lies wholly below the last of n words of x and a guard word,
	// leaving it out changes the sum by less than one unit in the last
	// place.
	if x.top()-y.top() > 64*n+64 {
		return mpNorm(n, x.m, x.exp, x.neg)
	}
	e := min(x.exp, y.exp)
	a := shlWords(x.m, uint(x.exp-e))
	b := shlWords(y.m, uint(y.exp-e))
	l := max(len(a), len(b)) + 1
	a = append(a, make([]uint64, l-len(a))...)
	b = append(b, make([]uint64, l-len(b))...)
	if x.neg == y.neg {
		var c uint64
		for i := range a {
			a[i], c = bits.Add64(a[i], b[i], c)
		}
		return mpNorm(n, a, e, x.neg)
	}
	neg := x.neg
	if cmpWords(a, b) < 0 {
		a, b = b, a
		neg = y.neg
	}
	var c uint64
	for i := range a {
		a[i], c = bits.Sub64(a[i], b[i], c)
	}
	return mpNorm(n, a, e, neg)
}

// mpSub returns x - y truncated to n words.
func mpSub(n int, x, y mpf) mpf { return mpAdd(n, x, mpNeg(y)) }

// mpCmpAbs compares |x| and |y|, both normalized.
func mpCmpAbs(x, y mpf) int {
	switch {
	case x.m == nil && y.m == nil:
		return 0
	case x.m == nil:
		return -1
	case y.m == nil:
		return 1
	case x.top() != y.top():
		if x.top() < y.top() {
			return -1
		}
		return 1
	}
	// The top bits are aligned, so the words are, from the top down.
	i, j := len(x.m)-1, len(y.m)-1
	for ; i >= 0 && j >= 0; i, j = i-1, j-1 {
		if x.m[i] != y.m[j] {
			if x.m[i] < y.m[j] {
				return -1
			}
			return 1
		}
	}
	for ; i >= 0; i-- {
		if x.m[i] != 0 {
			return 1
		}
	}
	for ; j >= 0; j-- {
		if y.m[j] != 0 {
			return -1
		}
	}
	return 0
}

// divWords returns the quotient of the integer u by the integer v, whose
// top bit is set (Knuth, The Art of Computer Programming, volume 2,
// section 4.3.1, algorithm D). u is overwritten.
func divWords(u, v []uint64) []uint64 {
	nv := len(v)
	if len(u) < nv {
		return nil
	}
	q := make([]uint64, len(u)-nv+1)
	if nv == 1 {
		var r uint64
		for i := len(u) - 1; i >= 0; i-- {
			q[i], r = bits.Div64(r, u[i], v[0])
		}
		return q
	}
	u = append(u, 0)
	vn1, vn2 := v[nv-1], v[nv-2]
	for j := len(q) - 1; j >= 0; j-- {
		// Estimate the quotient word from the top two words of what
		// is left of u and the top word of v, and correct it by v's
		// second word; it is then at most one too large.
		qhat := ^uint64(0)
		if ujn := u[j+nv]; ujn < vn1 {
			var rhat uint64
			qhat, rhat = bits.Div64(ujn, u[j+nv-1], vn1)
			for {
				hi, lo := bits.Mul64(qhat, vn2)
				if hi < rhat || hi == rhat && lo <= u[j+nv-2] {
					break
				}
				qhat--
				rhat += vn1
				if rhat < vn1 { // overflowed: the estimate is now small enough
					break
				}
			}
		}
		// Subtract qhat × v from u at word j.
		var borrow, carry uint64
		for i, x := range v {
			hi, lo := bits.Mul64(qhat, x)
			var c uint64
			lo, c = bits.Add64(lo, carry, 0)
			carry = hi + c
			u[j+i], borrow = bits.Sub64(u[j+i], lo, borrow)
		}
		u[j+nv], borrow = bits.Sub64(u[j+nv], carry, borrow)
		if borrow != 0 {
			// One too large: add v back.
			qhat--
			var c uint64
			for i, x := range v {
				u[j+i], c = bits.Add64(u[j+i], x, c)
			}
			u[j+nv] += c
		}
		q[j] = qhat
	}
	return q
}

// mpDiv returns x / y truncated to n words. y must be normalized and not
// zero.
func mpDiv(n int, x, y mpf) mpf {
	if x.m == nil {
		return mpf{}
	}
	// Scale the dividend by whole words so that the quotient has at
	// least n+1 of them.
	k := max(n+1+len(y.m)-len(x.m), 0)
	u := make([]uint64, k+len(x.m), k+len(x.m)+1)
	copy(u[k:], x.m)
	return mpNorm(n, divWords(u, y.m), x.exp-64*k-y.exp, x.neg != y.neg)
}

// mpSqrt returns the square root of x, which must be positive, to within
// four units in the last of n words.
func mpSqrt(n int, x mpf) mpf {
	// Newton's iteration s' = (s + x/s)/2 from a power of two at or
	// above the root and less than twice it. The relative error e of s
	// becomes e²/(2(1+e)): 1, 1/4, 1/40, then squared and halved each
	// time, so two steps and then one for each doubling of 5 bits
	// reach 64n bits; truncation leaves it within a few units.
	s := mpShift(mpWord(n, 1), (x.top()+1)>>1)
	for i := 3 + bits.Len(uint(64*n/5)); i > 0; i-- {
		s = mpShift(mpAdd(n, s, mpDiv(n, x, s)), -1)
	}
	return s
}

// mpPow10 returns 10^e to within two units in the last of n words.
func mpPow10(n int, e int) mpf {
	// Powers by squaring at one more word than asked for: each of the
	// at most 2×log2|e| products adds a unit of that precision and the
	// squarings double what is there, which stays below |e| units in
	// all, far less than one unit of n words for any exponent a decimal
	// format reaches.
	a := uint(e)
	if e < 0 {
		a = uint(-e)
	}
	r := mpWord(n+1, 1)
	if a > 0 {
		p := mpWord(n+1, 10)
		for i := bits.Len(a) - 1; i >= 0; i-- {
			r = mpMul(n+1, r, r)
			if a>>uint(i)&1 != 0 {
				r = mpMul(n+1, r, p)
			}
		}
	}
	if e < 0 {
		return mpDiv(n, mpWord(n+1, 1), r)
	}
	return mpNorm(n, r.m, r.exp, false)
}

// mpFloor returns the integer part of |x| as a 128-bit integer. near
// reports that the fraction is within 2^guard units of x's last bit of
// zero or of one, so that an x known only to that many units may lie on
// either side of an integer. ok is false if the integer part does not
// fit in 128 bits or x has fewer than guard+2 bits below the binary
// point.
func mpFloor(x mpf, guard int) (hi, lo uint64, near, ok bool) {
	fracBits := -x.exp
	if x.m == nil || fracBits < guard+2 || x.top() > 128 {
		return 0, 0, false, false
	}
	// word returns the 64 bits of x.m that start at bit position p.
	word := func(p int) uint64 {
		i, s := p>>6, uint(p&63)
		var w uint64
		if i < len(x.m) {
			w = x.m[i] >> s
		}
		if s != 0 && i+1 < len(x.m) {
			w |= x.m[i+1] << (64 - s)
		}
		return w
	}
	// The fraction is within 2^guard of zero if its bits from guard up
	// are all clear, and within 2^guard of one if they are all set.
	zeros, ones := true, true
	for p := guard; p < fracBits; p += 64 {
		w := word(p)
		if k := fracBits - p; k < 64 {
			w |= ^uint64(0) << uint(k) // bits past the point: make them ones,
			if w != ^uint64(0) {
				ones = false
			}
			w &^= ^uint64(0) << uint(k) // then zeros
			if w != 0 {
				zeros = false
			}
			break
		}
		if w != 0 {
			zeros = false
		}
		if w != ^uint64(0) {
			ones = false
		}
	}
	return word(fracBits + 64), word(fracBits), zeros || ones, true
}
