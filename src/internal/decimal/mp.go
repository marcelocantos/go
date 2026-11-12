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

// An arena is where the words of one evaluation's numbers live. Its
// store is an array on the stack of the function that starts the
// evaluation, so that an evaluation at the usual precision allocates
// nothing; a function that runs out of store takes its words from the
// heap instead, which is all that the precisions of a retry cost.
//
// The words are handed out in order and given back in order. A function
// here that has intermediate results notes where the arena stood when it
// was called (mark), and before it returns gives back everything since,
// leaving only its result (ret). A loop does the same for each turn, and
// keeps what the next turn needs in words it took before the loop
// (mpKeep). Every function leaves the arena holding its result and
// nothing else of its own, so a caller's numbers stay valid until the
// caller itself gives them back.
type arena struct {
	buf []uint64 // the words handed out; its capacity is the store
}

// words returns k words, which are not cleared.
func (a *arena) words(k int) []uint64 {
	l := len(a.buf)
	if l+k > cap(a.buf) {
		return make([]uint64, k)
	}
	a.buf = a.buf[:l+k]
	return a.buf[l : l+k : l+k]
}

// zeros returns k words that are zero.
func (a *arena) zeros(k int) []uint64 {
	w := a.words(k)
	clear(w)
	return w
}

// mark returns where the arena stands, for release and ret.
func (a *arena) mark() int { return len(a.buf) }

// release gives back the words handed out since the mark m.
func (a *arena) release(m int) { a.buf = a.buf[:m] }

// ret gives back the words handed out since the mark m, and returns x,
// which may be among them, in words of its own.
func (a *arena) ret(m int, x mpf) mpf {
	a.release(m)
	if x.m == nil {
		return x
	}
	w := a.words(len(x.m))
	copy(w, x.m) // the two may overlap, the copy then moving x down
	x.m = w
	return x
}

// mpKeep copies x, of at most len(slot) words, into slot, and returns
// it there.
func mpKeep(slot []uint64, x mpf) mpf {
	if x.m == nil {
		return x
	}
	s := slot[:len(x.m)]
	copy(s, x.m)
	x.m = s
	return x
}

// mpNormTo sets m, which must not overlap w, to the first len(m) words
// of the integer w, least significant word first and of any length, and
// returns (-1)^neg × w × 2^exp truncated to them.
func mpNormTo(m, w []uint64, exp int, neg bool) mpf {
	n, l := len(m), len(w)
	for l > 0 && w[l-1] == 0 {
		l--
	}
	if l == 0 {
		return mpf{}
	}
	lz := uint(bits.LeadingZeros64(w[l-1]))
	if d := l - n; d >= 1 {
		// The usual case, a product or a quotient: w is longer than m,
		// and every word of m has a word of w below it.
		s := w[d-1 : l]
		s = s[:len(m)+1]
		if lz == 0 {
			copy(m, s[1:])
		} else {
			for i := range m {
				m[i] = s[i+1]<<lz | s[i]>>(64-lz)
			}
		}
		return mpf{m, exp + 64*d - int(lz), neg}
	}
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

// mpNorm returns (-1)^neg × w × 2^exp truncated to n words, where w is an
// integer of any length, least significant word first.
func mpNorm(a *arena, n int, w []uint64, exp int, neg bool) mpf {
	return mpNormTo(a.words(n), w, exp, neg)
}

// mpWord returns the integer w as an mpf of n words.
func mpWord(a *arena, n int, w uint64) mpf {
	return mpNormTo(a.words(n), []uint64{w}, 0, false)
}

// mpInt returns the integer hi×2^64 + lo as an mpf of n words; it is
// exact for n >= 2.
func mpInt(a *arena, n int, hi, lo uint64) mpf {
	return mpNormTo(a.words(n), []uint64{lo, hi}, 0, false)
}

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
func mpMul(a *arena, n int, x, y mpf) mpf {
	if x.m == nil || y.m == nil {
		return mpf{}
	}
	r := a.words(n)
	m := a.mark()
	if len(x.m) == 3 && len(y.m) == 3 {
		// The precision of a first evaluation.
		w := a.words(6)
		mulWords3((*[6]uint64)(w), (*[3]uint64)(x.m), (*[3]uint64)(y.m))
		z := mpNormTo(r, w, x.exp+y.exp, x.neg != y.neg)
		a.release(m)
		return z
	}
	w := a.zeros(len(x.m) + len(y.m))
	for i, xi := range x.m {
		var c uint64
		for j, yj := range y.m {
			hi, lo := bits.Mul64(xi, yj)
			var c1, c2 uint64
			lo, c1 = bits.Add64(lo, w[i+j], 0)
			lo, c2 = bits.Add64(lo, c, 0)
			w[i+j] = lo
			c = hi + c1 + c2 // cannot overflow: xi×yj + two words < 2^128
		}
		w[i+len(y.m)] = c
	}
	z := mpNormTo(r, w, x.exp+y.exp, x.neg != y.neg)
	a.release(m)
	return z
}

// mulWords3 sets w to x × y.
func mulWords3(w *[6]uint64, x, y *[3]uint64) {
	// Each row is x[i] × y, three products whose low words go in at
	// word i and whose high words go in a word higher. A row's sum
	// cannot carry out of its top word: the total is below 2^384.
	var c uint64
	h0, l0 := bits.Mul64(x[0], y[0])
	h1, l1 := bits.Mul64(x[0], y[1])
	h2, l2 := bits.Mul64(x[0], y[2])
	w0 := l0
	w1, c := bits.Add64(l1, h0, 0)
	w2, c := bits.Add64(l2, h1, c)
	w3 := h2 + c

	h0, l0 = bits.Mul64(x[1], y[0])
	h1, l1 = bits.Mul64(x[1], y[1])
	h2, l2 = bits.Mul64(x[1], y[2])
	w1, c = bits.Add64(w1, l0, 0)
	w2, c = bits.Add64(w2, l1, c)
	w3, c = bits.Add64(w3, l2, c)
	w4 := c
	w2, c = bits.Add64(w2, h0, 0)
	w3, c = bits.Add64(w3, h1, c)
	w4 += h2 + c

	h0, l0 = bits.Mul64(x[2], y[0])
	h1, l1 = bits.Mul64(x[2], y[1])
	h2, l2 = bits.Mul64(x[2], y[2])
	w2, c = bits.Add64(w2, l0, 0)
	w3, c = bits.Add64(w3, l1, c)
	w4, c = bits.Add64(w4, l2, c)
	w5 := c
	w3, c = bits.Add64(w3, h0, 0)
	w4, c = bits.Add64(w4, h1, c)
	w5 += h2 + c

	w[0], w[1], w[2], w[3], w[4], w[5] = w0, w1, w2, w3, w4, w5
}

// mpMulWord returns x × w truncated to n words.
func mpMulWord(a *arena, n int, x mpf, w uint64) mpf {
	if x.m == nil || w == 0 {
		return mpf{}
	}
	r := a.words(n)
	m := a.mark()
	p := a.words(len(x.m) + 1)
	var c uint64
	for i, xi := range x.m {
		hi, lo := bits.Mul64(xi, w)
		var c1 uint64
		p[i], c1 = bits.Add64(lo, c, 0)
		c = hi + c1
	}
	p[len(x.m)] = c
	z := mpNormTo(r, p, x.exp, x.neg)
	a.release(m)
	return z
}

// mpDivWord returns x / w truncated to n words. w must not be zero.
func mpDivWord(a *arena, n int, x mpf, w uint64) mpf {
	if x.m == nil {
		return mpf{}
	}
	r := a.words(n)
	m := a.mark()
	// Divide x.m scaled by whole words, so that the quotient has a word
	// more than n whatever w is.
	k := max(n-len(x.m), 0) + 1
	q := a.words(len(x.m) + k)
	var rem uint64
	if w>>32 == 0 {
		// A divisor of a series, a small count: two machine divisions
		// a word, where bits.Div64 is a function of many steps.
		for i := len(x.m) - 1; i >= 0; i-- {
			q[i+k], rem = divSmall(rem, x.m[i], w)
		}
		for i := k - 1; i >= 0; i-- {
			q[i], rem = divSmall(rem, 0, w)
		}
	} else {
		for i := len(x.m) - 1; i >= 0; i-- {
			q[i+k], rem = bits.Div64(rem, x.m[i], w)
		}
		for i := k - 1; i >= 0; i-- {
			q[i], rem = bits.Div64(rem, 0, w)
		}
	}
	z := mpNormTo(r, q, x.exp-64*k, x.neg)
	a.release(m)
	return z
}

// divSmall returns the quotient and remainder of hi×2^64 + lo by d, for
// hi < d < 2^32.
func divSmall(hi, lo, d uint64) (q, r uint64) {
	t := hi<<32 | lo>>32
	qh, r := t/d, t%d
	t = r<<32 | lo&(1<<32-1)
	return qh<<32 | t/d, t % d
}

// shlWords sets r, which is zero and long enough, to w × 2^s.
func shlWords(r, w []uint64, s uint) {
	ws, bs := int(s/64), s%64
	for i, x := range w {
		r[i+ws] |= x << bs
		if bs != 0 {
			r[i+ws+1] = x >> (64 - bs)
		}
	}
}

// mpAdd returns x + y truncated to n words.
func mpAdd(a *arena, n int, x, y mpf) mpf {
	if x.m == nil {
		return mpNorm(a, n, y.m, y.exp, y.neg)
	}
	if y.m == nil {
		return mpNorm(a, n, x.m, x.exp, x.neg)
	}
	if x.top() < y.top() {
		x, y = y, x
	}
	// If y lies wholly below the last of n words of x and a guard word,
	// leaving it out changes the sum by less than one unit in the last
	// place.
	if x.top()-y.top() > 64*n+64 {
		return mpNorm(a, n, x.m, x.exp, x.neg)
	}
	// The sum of the magnitudes, or the larger less the smaller.
	sub := x.neg != y.neg
	if sub && mpCmpAbs(x, y) < 0 {
		x, y = y, x
	}
	r := a.words(n)
	m := a.mark()
	// x as an integer at the lesser exponent, with a word for the shift
	// and a word for the carry; y is added to it, or taken from it, a
	// word at a time as it is shifted.
	e := min(x.exp, y.exp)
	sx, sy := uint(x.exp-e), uint(y.exp-e)
	l := max(len(x.m)+int(sx/64), len(y.m)+int(sy/64)) + 2
	p := a.zeros(l)
	shlWords(p, x.m, sx)
	ws, bs := int(sy/64), sy%64
	var c, low uint64 // low is what the word below shifted into this one
	t := p[ws:]
	for i, w := range y.m {
		v := w<<bs | low
		low = 0
		if bs != 0 {
			low = w >> (64 - bs)
		}
		if sub {
			t[i], c = bits.Sub64(t[i], v, c)
		} else {
			t[i], c = bits.Add64(t[i], v, c)
		}
	}
	for i := len(y.m); c|low != 0; i++ {
		if sub {
			t[i], c = bits.Sub64(t[i], low, c)
		} else {
			t[i], c = bits.Add64(t[i], low, c)
		}
		low = 0
	}
	z := mpNormTo(r, p, e, x.neg)
	a.release(m)
	return z
}

// mpSub returns x - y truncated to n words.
func mpSub(a *arena, n int, x, y mpf) mpf { return mpAdd(a, n, x, mpNeg(y)) }

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

// reciprocal returns floor((2^128-1)/d) - 2^64, for a d whose top bit is
// set: the reciprocal that divRecip divides by (Möller and Granlund,
// Improved division by invariant integers, IEEE Transactions on
// Computers, 2011).
func reciprocal(d uint64) uint64 {
	v, _ := bits.Div64(^d, ^uint64(0), d)
	return v
}

// divRecip returns the quotient and remainder of hi×2^64 + lo by d,
// whose top bit is set and whose reciprocal is inv, for hi < d
// (algorithm 4 of the paper): two products where bits.Div64 divides.
func divRecip(hi, lo, d, inv uint64) (q, r uint64) {
	q, q0 := bits.Mul64(inv, hi)
	q0, c := bits.Add64(q0, lo, 0)
	q, _ = bits.Add64(q, hi, c)
	q++
	r = lo - q*d
	if r > q0 {
		q--
		r += d
	}
	if r >= d {
		q++
		r -= d
	}
	return q, r
}

// divWords sets q to the quotient of the integer u by the integer v,
// whose top bit is set (Knuth, The Art of Computer Programming, volume
// 2, section 4.3.1, algorithm D). The top word of u is zero, q has
// len(u)-len(v) words, and u is overwritten.
func divWords(q, u, v []uint64) {
	nv := len(v)
	inv := reciprocal(v[nv-1])
	if nv == 1 {
		var r uint64
		for i := len(q) - 1; i >= 0; i-- {
			q[i], r = divRecip(r, u[i], v[0], inv)
		}
		return
	}
	vn1, vn2 := v[nv-1], v[nv-2]
	for j := len(q) - 1; j >= 0; j-- {
		// Estimate the quotient word from the top two words of what
		// is left of u and the top word of v, and correct it by v's
		// second word; it is then at most one too large.
		qhat := ^uint64(0)
		if ujn := u[j+nv]; ujn < vn1 {
			var rhat uint64
			qhat, rhat = divRecip(ujn, u[j+nv-1], vn1, inv)
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
}

// mpDiv returns x / y truncated to n words. y must be normalized and not
// zero.
func mpDiv(a *arena, n int, x, y mpf) mpf {
	if x.m == nil {
		return mpf{}
	}
	r := a.words(n)
	m := a.mark()
	// Scale the dividend by whole words so that the quotient has at
	// least n+1 of them, and give it the zero top word the division
	// wants.
	k := max(n+1+len(y.m)-len(x.m), 0)
	u := a.zeros(k + len(x.m) + 1)
	copy(u[k:], x.m)
	q := a.words(len(u) - len(y.m))
	divWords(q, u, y.m)
	z := mpNormTo(r, q, x.exp-64*k-y.exp, x.neg != y.neg)
	a.release(m)
	return z
}

// mpSqrt returns the square root of x, which must be positive, to within
// four units in the last of n words.
func mpSqrt(a *arena, n int, x mpf) mpf {
	// Newton's iteration s' = (s + x/s)/2, whose relative error e
	// becomes e²/(2(1+e)), from the root of the top word of x: that is
	// right to about 30 bits, and each step doubles them. Truncation
	// leaves the last step within a few units.
	t, e := x.m[len(x.m)-1], x.exp+64*(len(x.m)-1)
	if e&1 != 0 {
		t, e = t>>1, e+1
	}
	// The integer root of t, at least 2^62: Newton from above.
	r := uint64(1) << 32
	for {
		q := (r + t/r) >> 1
		if q >= r {
			break
		}
		r = q
	}
	slot := a.words(n)
	s := mpShift(mpNormTo(slot, []uint64{r}, 0, false), e>>1)
	for i := bits.Len(uint(64*n/30)) + 1; i > 0; i-- {
		m := a.mark()
		s = mpKeep(slot, mpShift(mpAdd(a, n, s, mpDiv(a, n, x, s)), -1))
		a.release(m)
	}
	return s
}

// mpPow10 returns 10^e to within two units in the last of n words.
func mpPow10(a *arena, n int, e int) mpf {
	// Powers by squaring at one more word than asked for: each of the
	// at most 2×log2|e| products adds a unit of that precision and the
	// squarings double what is there, which stays below |e| units in
	// all, far less than one unit of n words for any exponent a decimal
	// format reaches.
	u := uint(e)
	if e < 0 {
		u = uint(-e)
	}
	m0 := a.mark()
	if u < uint(len(pow10Word)) {
		// A power that is one word: exact, or one division.
		if e < 0 {
			return a.ret(m0, mpDivWord(a, n, mpWord(a, n, 1), pow10Word[u]))
		}
		return mpWord(a, n, pow10Word[u])
	}
	slot := a.words(n + 1)
	r := mpNormTo(slot, []uint64{1}, 0, false)
	p := mpWord(a, n+1, 10)
	for i := bits.Len(u) - 1; i >= 0; i-- {
		m := a.mark()
		t := mpMul(a, n+1, r, r)
		if u>>uint(i)&1 != 0 {
			t = mpMul(a, n+1, t, p)
		}
		r = mpKeep(slot, t)
		a.release(m)
	}
	if e < 0 {
		return a.ret(m0, mpDiv(a, n, mpWord(a, n+1, 1), r))
	}
	return a.ret(m0, mpNorm(a, n, r.m, r.exp, false))
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
