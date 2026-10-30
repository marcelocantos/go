// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import (
	"math"
	"math/big"
	"testing"
)

// Tests of the decimal-digit primitives against math/big. The oracle takes
// nothing from the package: its powers of ten come from big.Int.Exp, and its
// classification compares 2r with 10^k, so a wrong table or a wrong half
// cannot agree with itself.

// digitWidth adapts one width of the primitives to big.Int. Every value
// passed in must fit the width.
type digitWidth struct {
	name   string
	bits   uint
	tab    int // number of powers of ten below 2^bits
	digits func(c *big.Int) int
	shift  func(c *big.Int, k int) (*big.Int, digitClass)
	scale  func(c *big.Int, k int) (*big.Int, bool)
	// reduce is the shift whose quotient must fit half the width, or nil.
	reduce func(c *big.Int, k int) (*big.Int, digitClass)
}

func digitWidths() []digitWidth {
	return []digitWidth{
		{
			name: "64", bits: 64, tab: len(pow10tab64),
			digits: func(c *big.Int) int { return cDigits64(c.Uint64()) },
			shift: func(c *big.Int, k int) (*big.Int, digitClass) {
				q, class := cShift64(c.Uint64(), k)
				return big64(q), class
			},
			scale: func(c *big.Int, k int) (*big.Int, bool) {
				r, o := cScale64(c.Uint64(), k)
				return big64(r), o
			},
		},
		{
			name: "128", bits: 128, tab: len(pow10tab128),
			digits: func(c *big.Int) int { return cDigits128(from128(c)) },
			shift: func(c *big.Int, k int) (*big.Int, digitClass) {
				q, class := cShift128(from128(c), k)
				return bigOf(q), class
			},
			scale: func(c *big.Int, k int) (*big.Int, bool) {
				r, o := cScale128(from128(c), k)
				return bigOf(r), o
			},
			reduce: func(c *big.Int, k int) (*big.Int, digitClass) {
				q, class := cReduceClass64(from128(c), k)
				return big64(q), class
			},
		},
		{
			name: "256", bits: 256, tab: len(pow10tab256),
			digits: func(c *big.Int) int { return cLenFull128(from256(c)) },
			shift: func(c *big.Int, k int) (*big.Int, digitClass) {
				q, class := cShiftFull128(from256(c), k)
				return bigOf256(q), class
			},
			scale: func(c *big.Int, k int) (*big.Int, bool) {
				r, o := cScaleFull128(from256(c), k)
				return bigOf256(r), o
			},
			reduce: func(c *big.Int, k int) (*big.Int, digitClass) {
				q, class := cReduceClass128(from256(c), k)
				return bigOf(q), class
			},
		},
	}
}

// fits reports whether c can be passed to the width.
func (w *digitWidth) fits(c *big.Int) bool {
	return c.Sign() >= 0 && uint(c.BitLen()) <= w.bits
}

// keep returns the values of vs that fit the width.
func (w *digitWidth) keep(vs []*big.Int) []*big.Int {
	var out []*big.Int
	for _, v := range vs {
		if w.fits(v) {
			out = append(out, v)
		}
	}
	return out
}

// maxOracleK bounds the exponent that the oracle computes with: every value
// tested is far below 10^maxOracleK, so a larger k has the same answer as
// this one.
const maxOracleK = 200

// wantShift is the oracle for a shift: the quotient, the remainder and the
// class of the remainder.
func wantShift(c *big.Int, k int) (q, r *big.Int, class digitClass) {
	k = min(k, maxOracleK)
	p := bigPow10(k)
	q, r = new(big.Int).QuoRem(c, p, new(big.Int))
	if r.Sign() == 0 {
		return q, r, classZero
	}
	switch new(big.Int).Lsh(r, 1).Cmp(p) {
	case -1:
		return q, r, classBelowHalf
	case 0:
		return q, r, classHalf
	}
	return q, r, classAboveHalf
}

// digitBoundaries returns the values around each power of ten, and those
// around its half, up to the largest that 256 bits holds, and around each
// power of two. Callers filter by width.
func digitBoundaries() []*big.Int {
	var vs []*big.Int
	add := func(v *big.Int) {
		vs = append(vs, v, new(big.Int).Add(v, bigOne))
		if v.Sign() > 0 {
			vs = append(vs, new(big.Int).Sub(v, bigOne))
		}
	}
	for j := 0; j <= 78; j++ {
		add(bigPow10(j)) // 10^j - 1 is the all-nines value of j digits
		if j > 0 {
			add(new(big.Int).Mul(big.NewInt(5), bigPow10(j-1)))
		}
	}
	for b := uint(0); b <= 256; b++ {
		add(new(big.Int).Lsh(bigOne, b))
	}
	return vs
}

// tailsAt returns values with a non-zero quotient at k whose tail is on or
// next to each classification boundary, for k >= 1.
func tailsAt(w *digitWidth, k int) []*big.Int {
	p := bigPow10(k)
	half := new(big.Int).Rsh(p, 1)
	tails := []*big.Int{
		big.NewInt(0), bigOne,
		new(big.Int).Sub(half, bigOne), half, new(big.Int).Add(half, bigOne),
		new(big.Int).Sub(p, bigOne),
	}
	limit := new(big.Int).Lsh(bigOne, w.bits)
	top := new(big.Int).Div(limit, p) // the largest quotient that can fit, or one more
	var vs []*big.Int
	for _, m := range []*big.Int{bigOne, big.NewInt(2), big.NewInt(3), top, new(big.Int).Sub(top, bigOne)} {
		for _, t := range tails {
			v := new(big.Int).Mul(m, p)
			vs = append(vs, v.Add(v, t))
		}
	}
	return w.keep(vs)
}

// checkDigitValue checks every primitive of the width on c at k.
func checkDigitValue(t *testing.T, w *digitWidth, c *big.Int, k int) {
	t.Helper()
	wantQ, wantR, wantClass := wantShift(c, k)
	q, class := w.shift(c, k)
	if q.Cmp(wantQ) != 0 || class != wantClass {
		t.Errorf("shift%s(%v, %d) = %v, %d, want %v, %d", w.name, c, k, q, class, wantQ, wantClass)
	}

	// The narrowing form agrees whenever its quotient fits, and panics
	// otherwise.
	if w.reduce != nil {
		if uint(wantQ.BitLen()) <= w.bits/2 {
			q, class := w.reduce(c, k)
			if q.Cmp(wantQ) != 0 || class != wantClass {
				t.Errorf("reduce%s(%v, %d) = %v, %d, want %v, %d", w.name, c, k, q, class, wantQ, wantClass)
			}
		} else {
			mustPanic(t, "reduce"+w.name, func() { w.reduce(c, k) })
		}
	}

	// An incoming sticky bit, for fractions on both sides of 1/2. Above
	// k = 0 the class must be that of the exact c + f.
	if k >= 1 {
		for _, f := range [][2]int64{{1, 1000}, {1, 2}, {999, 1000}} {
			num := new(big.Int).Mul(wantR, big.NewInt(f[1]))
			num.Add(num, big.NewInt(f[0]))
			num.Lsh(num, 1)
			den := new(big.Int).Mul(bigPow10(min(k, maxOracleK)), big.NewInt(f[1]))
			want := classAboveHalf
			switch num.Cmp(den) {
			case -1:
				want = classBelowHalf
			case 0:
				want = classHalf
			}
			if got := class.withSticky(true); got != want {
				t.Errorf("shift%s(%v, %d) class %d withSticky(%d/%d) = %d, want %d", w.name, c, k, class, f[0], f[1], got, want)
			}
		}
	} else if got := class.withSticky(true); got != classBelowHalf {
		// At k = 0 sticky is the ε of "Rounding from exact parts".
		t.Errorf("shift%s(%v, 0) class %d withSticky = %d, want classBelowHalf", w.name, c, class, got)
	}
	if got := class.withSticky(false); got != class {
		t.Errorf("class %d withSticky(false) = %d", class, got)
	}

	// Scaling, and its inverse.
	product := new(big.Int)
	if k <= maxOracleK {
		product.Mul(c, bigPow10(k))
	} else if c.Sign() != 0 {
		product.Lsh(bigOne, w.bits) // too large, whatever k is
	}
	r, overflow := w.scale(c, k)
	if w.fits(product) {
		if overflow || r.Cmp(product) != 0 {
			t.Errorf("scale%s(%v, %d) = %v, %t, want %v", w.name, c, k, r, overflow, product)
		}
		if q, class := w.shift(r, k); q.Cmp(c) != 0 || class != classZero {
			t.Errorf("shift%s(scale(%v, %d)) = %v, %d", w.name, c, k, q, class)
		}
	} else if !overflow || r.Sign() != 0 {
		t.Errorf("scale%s(%v, %d) = %v, %t, want 0, true", w.name, c, k, r, overflow)
	}
}

// checkDigitSplit checks that shifting by a and then by b is shifting by
// a + b, with the first class reaching the second only through withSticky.
// This is what makes a path with several exact steps round once.
func checkDigitSplit(t *testing.T, w *digitWidth, c *big.Int, a, b int) {
	t.Helper()
	q1, class1 := w.shift(c, a)
	q2, class2 := w.shift(q1, b)
	q, class := w.shift(c, a+b)
	if got := class2.withSticky(class1 != classZero); q2.Cmp(q) != 0 || got != class {
		t.Errorf("shift%s(%v) by %d then %d = %v, %d, want %v, %d", w.name, c, a, b, q2, got, q, class)
	}
}

// TestDigitShiftSweep is the exhaustive sweep of k for each width: every k
// through the table and past it, on the boundary values of every power of
// ten and of two, and on tails placed on each classification boundary at
// that k with a non-zero quotient.
func TestDigitShiftSweep(t *testing.T) {
	boundaries := digitBoundaries()
	for _, w := range digitWidths() {
		t.Run(w.name, func(t *testing.T) {
			vs := w.keep(boundaries)
			maxV := new(big.Int).Sub(new(big.Int).Lsh(bigOne, w.bits), bigOne)
			vs = append(vs, big.NewInt(0), maxV, new(big.Int).Sub(maxV, bigOne))
			ks := []int{w.tab + 1000, 1 << 20, math.MaxInt}
			for k := 0; k <= w.tab+3; k++ {
				ks = append(ks, k)
			}
			for _, k := range ks {
				for _, c := range vs {
					checkDigitValue(t, &w, c, k)
				}
				if k >= 1 && k <= w.tab {
					for _, c := range tailsAt(&w, k) {
						checkDigitValue(t, &w, c, k)
					}
				}
			}
		})
	}
}

// TestDigitWideForwarders checks that the double-width forms of the decimal64
// vocabulary are the 128-bit forms.
func TestDigitWideForwarders(t *testing.T) {
	for _, p := range edge128() {
		for k := 0; k <= len(pow10tab128)+1; k++ {
			q, class := cShiftFull64(p, k)
			wq, wclass := cShift128(p, k)
			if q != wq || class != wclass {
				t.Errorf("cShiftFull64(%#x:%#x, %d) = %v, %d, want %v, %d", p.Hi, p.Lo, k, q, class, wq, wclass)
			}
			r, o := cScaleFull64(p, k)
			wr, wo := cScale128(p, k)
			if r != wr || o != wo {
				t.Errorf("cScaleFull64(%#x:%#x, %d) = %v, %t, want %v, %t", p.Hi, p.Lo, k, r, o, wr, wo)
			}
		}
	}
}

// TestDigitShiftClasses pins the classification at each k with values built
// to have each class, so that the sweep cannot pass by never reaching one.
func TestDigitShiftClasses(t *testing.T) {
	for _, w := range digitWidths() {
		for k := 1; k < w.tab; k++ {
			p := bigPow10(k)
			half := new(big.Int).Rsh(p, 1)
			for _, tt := range []struct {
				c    *big.Int
				want digitClass
			}{
				{p, classZero},
				{new(big.Int).Sub(half, bigOne), classBelowHalf},
				{half, classHalf},
				{new(big.Int).Add(half, bigOne), classAboveHalf},
				{new(big.Int).Sub(p, bigOne), classAboveHalf},
			} {
				if _, class := w.shift(tt.c, k); class != tt.want {
					t.Errorf("shift%s(%v, %d) class = %d, want %d", w.name, tt.c, k, class, tt.want)
				}
			}
		}
	}
}

// TestDigitShiftEdges covers each case named in "Decimal-digit primitives".
func TestDigitShiftEdges(t *testing.T) {
	for _, w := range digitWidths() {
		// k = 0 is the identity with an empty tail.
		for _, c := range w.keep([]*big.Int{big.NewInt(0), bigOne, big.NewInt(5), bigPow10(18)}) {
			if q, class := w.shift(c, 0); q.Cmp(c) != 0 || class != classZero {
				t.Errorf("shift%s(%v, 0) = %v, %d", w.name, c, q, class)
			}
		}
		// k one past the digit count is below half however large the digits.
		for d := 1; d < w.tab; d++ {
			c := new(big.Int).Sub(bigPow10(d), bigOne)
			if q, class := w.shift(c, d+1); q.Sign() != 0 || class != classBelowHalf {
				t.Errorf("shift%s(%v, %d) = %v, %d, want 0, below", w.name, c, d+1, q, class)
			}
		}
		// The first k past the table is below half for the largest value,
		// and that is true of the width: 2^bits < 5 × 10^(tab-1).
		maxV := new(big.Int).Sub(new(big.Int).Lsh(bigOne, w.bits), bigOne)
		if new(big.Int).Lsh(bigOne, w.bits).Cmp(new(big.Int).Mul(big.NewInt(5), bigPow10(w.tab-1))) >= 0 {
			t.Errorf("2^%d is not below half of 10^%d", w.bits, w.tab)
		}
		for _, k := range []int{w.tab, w.tab + 1, 1 << 30, math.MaxInt} {
			if q, class := w.shift(maxV, k); q.Sign() != 0 || class != classBelowHalf {
				t.Errorf("shift%s(max, %d) = %v, %d", w.name, k, q, class)
			}
			if q, class := w.shift(big.NewInt(0), k); q.Sign() != 0 || class != classZero {
				t.Errorf("shift%s(0, %d) = %v, %d", w.name, k, q, class)
			}
			if r, o := w.scale(big.NewInt(0), k); r.Sign() != 0 || o {
				t.Errorf("scale%s(0, %d) = %v, %t", w.name, k, r, o)
			}
			if r, o := w.scale(bigOne, k); r.Sign() != 0 || !o {
				t.Errorf("scale%s(1, %d) = %v, %t", w.name, k, r, o)
			}
		}
		// A negative k is a caller's bug and must not pass for anything.
		for _, c := range []*big.Int{big.NewInt(0), bigOne} {
			mustPanic(t, "shift"+w.name, func() { w.shift(c, -1) })
			mustPanic(t, "scale"+w.name, func() { w.scale(c, -1) })
			if w.reduce != nil {
				mustPanic(t, "reduce"+w.name, func() { w.reduce(c, -1) })
			}
		}
	}
}

// TestDigitShiftSplit checks every split of every k on a value of each
// shape: the digit path may take any number of exact steps and still have
// one answer.
func TestDigitShiftSplit(t *testing.T) {
	for _, w := range digitWidths() {
		for k := 1; k < w.tab+2; k++ {
			vs := []*big.Int{new(big.Int).Sub(new(big.Int).Lsh(bigOne, w.bits), bigOne)}
			if k <= w.tab {
				vs = append(vs, tailsAt(&w, k)...)
			}
			for _, c := range vs {
				for b := 1; b <= k; b++ {
					checkDigitSplit(t, &w, c, k-b, b)
				}
			}
		}
	}
}

// TestDigitClassSticky is the combination's truth table, stated directly.
func TestDigitClassSticky(t *testing.T) {
	for _, tt := range []struct{ in, want digitClass }{
		{classZero, classBelowHalf},
		{classBelowHalf, classBelowHalf},
		{classHalf, classAboveHalf},
		{classAboveHalf, classAboveHalf},
	} {
		if got := tt.in.withSticky(true); got != tt.want {
			t.Errorf("%d.withSticky(true) = %d, want %d", tt.in, got, tt.want)
		}
		if got := tt.in.withSticky(false); got != tt.in {
			t.Errorf("%d.withSticky(false) = %d", tt.in, got)
		}
	}
}

// TestDigitCountEstimate proves the digit-count method for every bit length
// up to the widest: the estimate from the bit length is never too large and
// at most one too small, so the one comparison the method makes is enough,
// and the index it makes is always in the table.
func TestDigitCountEstimate(t *testing.T) {
	for b := 1; b <= 256; b++ {
		est := b * 1233 >> 12
		lo := new(big.Int).Lsh(bigOne, uint(b-1))
		hi := new(big.Int).Sub(new(big.Int).Lsh(bigOne, uint(b)), bigOne)
		if d := bigDigits(lo); d != est && d != est+1 {
			t.Errorf("bit length %d: estimate %d, smallest value has %d digits", b, est, d)
		}
		if d := bigDigits(hi); d != est && d != est+1 {
			t.Errorf("bit length %d: estimate %d, largest value has %d digits", b, est, d)
		}
	}
	if last := 64 * 1233 >> 12; last >= len(pow10tab64) {
		t.Errorf("64-bit estimate %d is off the table", last)
	}
	if last := 128 * 1233 >> 12; last >= len(pow10tab128) {
		t.Errorf("128-bit estimate %d is off the table", last)
	}
	if last := 256 * 1233 >> 12; last >= len(pow10tab256) {
		t.Errorf("256-bit estimate %d is off the table", last)
	}
}

// TestDigitCount checks the digit count of each width on the values where it
// changes and at every bit length.
func TestDigitCount(t *testing.T) {
	boundaries := digitBoundaries()
	for _, w := range digitWidths() {
		vs := w.keep(boundaries)
		vs = append(vs, big.NewInt(0), new(big.Int).Sub(new(big.Int).Lsh(bigOne, w.bits), bigOne))
		for _, c := range vs {
			if got, want := w.digits(c), bigDigits(c); got != want {
				t.Errorf("digits%s(%v) = %d, want %d", w.name, c, got, want)
			}
		}
	}
}

// TestPow10Through68 pins the reach of the widest table: the product of two
// decimal128 coefficients is below 10^68, and the table must reach that.
// TestPow10Tables recomputes every entry.
func TestPow10Through68(t *testing.T) {
	if len(pow10tab256) <= 68 {
		t.Fatalf("pow10tab256 has %d entries, want at least 69", len(pow10tab256))
	}
	if got, want := bigOf256(pow10tab256[68]), bigPow10(68); got.Cmp(want) != 0 {
		t.Errorf("pow10tab256[68] = %v, want %v", got, want)
	}
}

// digitFuzz runs the checks on a raw input and on one built from it to land
// on a classification boundary, which random bits almost never do.
func digitFuzz(t *testing.T, w *digitWidth, raw *big.Int, k int, tail uint8, a int) {
	checkDigitValue(t, w, raw, k)
	if k >= 1 && k <= w.tab {
		first := a % k
		checkDigitSplit(t, w, raw, first, k-first)
		p := bigPow10(k)
		half := new(big.Int).Rsh(p, 1)
		var offset *big.Int
		switch tail % 6 {
		case 0:
			offset = big.NewInt(0)
		case 1:
			offset = bigOne
		case 2:
			offset = new(big.Int).Sub(half, bigOne)
		case 3:
			offset = half
		case 4:
			offset = new(big.Int).Add(half, bigOne)
		default:
			offset = new(big.Int).Sub(p, bigOne)
		}
		// Replace the digits below k with the chosen tail. The result can
		// pass the top of the width only by the tail, so step down if so.
		c := new(big.Int).Mul(new(big.Int).Div(raw, p), p)
		c.Add(c, offset)
		if !w.fits(c) {
			c.Sub(c, p)
		}
		if w.fits(c) {
			checkDigitValue(t, w, c, k)
		}
	}
}

func digitFuzzK(k uint16, w *digitWidth) int {
	// Most of the range falls within the table and just past it; the rest
	// exercises a k far beyond it.
	if k&0x8000 != 0 {
		return int(k &^ 0x8000)
	}
	return int(k) % (w.tab + 3)
}

func FuzzDigits64(f *testing.F) {
	w := digitWidths()[0]
	for _, c := range w.keep(digitBoundaries()) {
		f.Add(c.Uint64(), uint16(cDigits64(c.Uint64())), uint8(3), 1)
	}
	f.Add(uint64(math.MaxUint64), uint16(19), uint8(3), 7)
	f.Fuzz(func(t *testing.T, c uint64, k uint16, tail uint8, a int) {
		digitFuzz(t, &w, big64(c), digitFuzzK(k, &w), tail, max(a, 0))
	})
}

func FuzzDigits128(f *testing.F) {
	w := digitWidths()[1]
	for _, c := range w.keep(digitBoundaries()) {
		v := from128(c)
		f.Add(v.Hi, v.Lo, uint16(cDigits128(v)), uint8(3), 1)
	}
	f.Add(uint64(math.MaxUint64), uint64(math.MaxUint64), uint16(38), uint8(3), 7)
	f.Fuzz(func(t *testing.T, hi, lo uint64, k uint16, tail uint8, a int) {
		digitFuzz(t, &w, bigOf(Uint128{Hi: hi, Lo: lo}), digitFuzzK(k, &w), tail, max(a, 0))
	})
}

func FuzzDigits256(f *testing.F) {
	w := digitWidths()[2]
	for _, c := range w.keep(digitBoundaries()) {
		v := from256(c)
		f.Add(v.Hi.Hi, v.Hi.Lo, v.Lo.Hi, v.Lo.Lo, uint16(cLenFull128(v)), uint8(3), 1)
	}
	f.Add(uint64(math.MaxUint64), uint64(math.MaxUint64), uint64(math.MaxUint64), uint64(math.MaxUint64), uint16(77), uint8(3), 7)
	f.Fuzz(func(t *testing.T, w3, w2, w1, w0 uint64, k uint16, tail uint8, a int) {
		c := Uint256{Hi: Uint128{Hi: w3, Lo: w2}, Lo: Uint128{Hi: w1, Lo: w0}}
		digitFuzz(t, &w, bigOf256(c), digitFuzzK(k, &w), tail, max(a, 0))
	})
}

// TestDigitZeroAllocs asserts that no primitive allocates, across the k at
// which each changes method.
func TestDigitZeroAllocs(t *testing.T) {
	a := Uint128{Hi: 0x0123456789ABCDEF, Lo: 0xFEDCBA9876543210}
	w := Uint256{Hi: a, Lo: a}
	var sinkClass digitClass
	tests := []struct {
		name string
		f    func()
	}{
		{"cShift64", func() { sinkU64, sinkClass = cShift64(a.Lo, 7) }},
		{"cShift64 past table", func() { sinkU64, sinkClass = cShift64(a.Lo, 25) }},
		{"cShiftFull64", func() { sinkU128, sinkClass = cShiftFull64(a, 25) }},
		{"cReduceClass64", func() { sinkU64, sinkClass = cReduceClass64(Uint128{Lo: a.Lo}, 7) }},
		{"cReduceClass64 past table", func() { sinkU64, sinkClass = cReduceClass64(a, 25) }},
		{"cScale64", func() { sinkU64, sinkBool = cScale64(123, 7) }},
		{"cScaleFull64", func() { sinkU128, sinkBool = cScaleFull64(a, 3) }},
		{"cShift128", func() { sinkU128, sinkClass = cShift128(a, 25) }},
		{"cShift128 past table", func() { sinkU128, sinkClass = cShift128(a, 45) }},
		{"cShiftFull128", func() { sinkU256, sinkClass = cShiftFull128(w, 25) }},
		{"cShiftFull128 one step past", func() { sinkU256, sinkClass = cShiftFull128(w, 45) }},
		{"cShiftFull128 two steps past", func() { sinkU256, sinkClass = cShiftFull128(w, 77) }},
		{"cShiftFull128 past table", func() { sinkU256, sinkClass = cShiftFull128(w, 90) }},
		{"cReduceClass128", func() { sinkU128, sinkClass = cReduceClass128(w, 60) }},
		{"cScale128", func() { sinkU128, sinkBool = cScale128(Uint128{Lo: 123}, 25) }},
		{"cScaleFull128", func() { sinkU256, sinkBool = cScaleFull128(Uint256{Lo: a}, 25) }},
		{"cScaleFull128 wide power", func() { sinkU256, sinkBool = cScaleFull128(Uint256{Lo: Uint128{Lo: 7}}, 60) }},
		{"withSticky", func() { sinkClass = sinkClass.withSticky(sinkBool) }},
	}
	for _, tt := range tests {
		if got := testing.AllocsPerRun(100, tt.f); got != 0 {
			t.Errorf("%s allocates %v times per call, want 0", tt.name, got)
		}
	}
	_ = sinkClass
}
