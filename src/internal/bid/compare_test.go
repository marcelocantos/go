// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Tests of the comparisons, TotalOrder and Key beyond the differential
// tests of TestKernel: the cases of the strawman (test/decimal.go and the
// edge cases of the runtime's comparison, which the earlier audit checked),
// the layout and sign of Key, allocation, and properties checked on random
// samples: Key is equal whenever Equal is true, and TotalOrder is a total
// order. The expected results here are written down from the contract, not
// computed by the reference model, except in the property tests, which
// build their operands with the model's encoder and judge them by
// relations that need no model.

import (
	"fmt"
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
	"unsafe"
)

// opnd is one decimal opnd of a table: a finite value c × 10^e, or
// one of the special values.
type opnd struct {
	kind bid.Kind
	neg  bool
	c    string // decimal digits of the coefficient
	e    int
	snan bool
}

func fin(c string, e int) opnd  { return opnd{c: c, e: e} }
func nfin(c string, e int) opnd { return opnd{neg: true, c: c, e: e} }

var (
	cNaN    = opnd{kind: bid.KindNaN}
	cSNaN   = opnd{kind: bid.KindNaN, snan: true}
	cNegNaN = opnd{kind: bid.KindNaN, neg: true}
	cInf    = opnd{kind: bid.KindInf}
	cNegInf = opnd{kind: bid.KindInf, neg: true}
)

func (o opnd) String() string {
	switch {
	case o.kind == bid.KindNaN && o.snan:
		return "sNaN"
	case o.kind == bid.KindNaN && o.neg:
		return "-NaN"
	case o.kind == bid.KindNaN:
		return "NaN"
	case o.kind == bid.KindInf && o.neg:
		return "-Inf"
	case o.kind == bid.KindInf:
		return "Inf"
	case o.neg:
		return fmt.Sprintf("-%sE%d", o.c, o.e)
	}
	return fmt.Sprintf("%sE%d", o.c, o.e)
}

// bits encodes o in format f.
func (o opnd) bits(f *format) bits {
	switch o.kind {
	case bid.KindNaN:
		x := f.encode(value{kind: notANumber})
		if o.snan {
			x = f.setSignaling(x)
		}
		return f.withSign(x, o.neg)
	case bid.KindInf:
		return f.encode(value{kind: infinite, neg: o.neg})
	}
	c, ok := new(big.Int).SetString(o.c, 10)
	if !ok {
		panic("bad coefficient " + o.c)
	}
	return f.encode(value{kind: finite, neg: o.neg, coeff: c, exp: o.e})
}

// setSignaling sets the signaling bit of the NaN x.
func (f *format) setSignaling(x bits) bits {
	if f.width == 64 {
		x.Lo |= 1 << 57
	} else {
		x.Hi |= 1 << 57
	}
	return x
}

const unordered = 2 // the result of a comparison with a NaN opnd

// compareCase is a pair of operands and the contract's result for them:
// -1, 0 or +1, or unordered.
type compareCase struct {
	x, y opnd
	want int
}

// strawmanCases are the comparisons of test/decimal.go (1 and 2, -5 and 1,
// NaN against 1 and itself, -0 against +0, the infinities against finite
// values and each other) and of the strawman's helpers: values of one
// cohort, exponents far enough apart that scaling would overflow or leave
// the table of powers of ten, and adjusted exponents that coincide with
// different quanta. They are valid in both formats: the largest exponent
// used is 300 and the smallest -300, and coefficients have 16 digits or
// fewer.
var strawmanCases = []compareCase{
	{fin("1", 0), fin("1", 0), 0},
	{fin("1", 0), fin("2", 0), -1},
	{fin("2", 0), fin("1", 0), 1},
	{nfin("5", 0), fin("1", 0), -1},
	{fin("1", 0), nfin("5", 0), 1},
	{nfin("5", 0), nfin("7", 0), 1},
	{nfin("7", 0), nfin("5", 0), -1},

	{cNaN, fin("1", 0), unordered},
	{fin("1", 0), cNaN, unordered},
	{cNaN, cNaN, unordered},
	{cSNaN, fin("1", 0), unordered},
	{cSNaN, cSNaN, unordered},
	{cNegNaN, cInf, unordered},
	{cNegInf, cNaN, unordered},
	{cNaN, fin("0", 0), unordered},

	{fin("0", 0), nfin("0", 0), 0},
	{nfin("0", 0), fin("0", 0), 0},
	{fin("0", 5), nfin("0", -5), 0},
	{fin("0", 300), fin("0", -300), 0},
	{fin("0", 0), fin("1", -300), -1},
	{nfin("0", 0), fin("1", -300), -1},
	{fin("0", 300), nfin("1", 300), 1},
	{nfin("0", -300), fin("1", 300), -1},

	{cInf, fin("999999", 0), 1},
	{cNegInf, nfin("999999", 0), -1},
	{cInf, cInf, 0},
	{cNegInf, cNegInf, 0},
	{cNegInf, cInf, -1},
	{cInf, cNegInf, 1},
	{cInf, fin("9999999999999999", 300), 1},
	{cNegInf, nfin("9999999999999999", 300), -1},
	{cInf, fin("0", 0), 1},
	{cNegInf, fin("0", 0), -1},
	{fin("0", 0), cInf, -1},
	{nfin("0", 0), cNegInf, 1},

	// One cohort.
	{fin("1", 0), fin("10", -1), 0},
	{fin("1", 0), fin("1000000000000000", -15), 0},
	{fin("100", 0), fin("1", 2), 0},
	{fin("1200", -2), fin("12", 0), 0},
	{nfin("1200", -2), nfin("12", 0), 0},
	{fin("12", 0), fin("1200", -2), 0},
	{fin("1", 3), fin("1000", 0), 0},
	{nfin("1", 3), nfin("1000", 0), 0},

	// Equal exponents: the coefficients decide.
	{fin("123456789", -5), fin("123456790", -5), -1},
	{nfin("123456789", -5), nfin("123456790", -5), 1},

	// Different exponents and values.
	{fin("1", 0), fin("11", -1), -1},
	{fin("11", -1), fin("1", 0), 1},
	{fin("1", 0), fin("9", -1), 1},
	{fin("9999999999999999", 0), fin("1", 16), -1},
	{fin("1", 16), fin("9999999999999999", 0), 1},
	{fin("9999999999999999", 0), fin("1000000000000000", 1), -1},
	{fin("1000000000000000", 1), fin("9999999999999999", 0), 1},
	{nfin("9999999999999999", 0), nfin("1", 16), 1},
	{fin("1", 15), fin("9999999999999999", -1), 1},
	{fin("9999999999999999", -1), fin("1", 15), -1},

	// Exponents too far apart to scale: the adjusted exponent decides, and
	// nothing overflows or indexes past the table of powers of ten.
	{fin("1", 300), fin("9999999999999999", -300), 1},
	{fin("9999999999999999", -300), fin("1", 300), -1},
	{nfin("1", 300), nfin("9999999999999999", -300), -1},
	{nfin("9999999999999999", -300), nfin("1", 300), 1},
	{fin("1", 100), fin("1", 0), 1},
	{fin("1", 20), fin("9999999999999999", 0), 1}, // 10^20 against about 10^16
	{fin("1", 17), fin("9999999999999999", 0), 1}, // exponents 17 apart
	{fin("1", 16), fin("9999999999999999", 0), 1}, // adjusted exponents 16 and 15
	{fin("9999999999999999", 1), fin("1", 17), -1},
	{fin("1", 18), fin("1", 1), 1},
	{fin("1", 19), fin("1", 0), 1},
	{fin("1", 20), fin("1", 0), 1},
	{fin("5", -300), fin("1", -299), -1},

	// Equal adjusted exponents, different quanta, different values.
	{fin("1", 15), fin("10000000000000001", -1), -1},
	{fin("15", 0), fin("1500000000000001", -14), -1},
	{fin("1500000000000001", -14), fin("15", 0), 1},
	{fin("9", 0), fin("90000000000001", -13), -1},
}

func TestStrawmanComparisons(t *testing.T) {
	for _, f := range both {
		for _, tc := range strawmanCases {
			if f == dec64 && len(tc.x.c) > 16 || f == dec64 && len(tc.y.c) > 16 {
				continue // an opnd beyond decimal64's precision
			}
			x, y := tc.x.bits(f), tc.y.bits(f)
			check := func(im impl, name string) {
				c, ok := im.compare(f, x, y)
				if tc.want == unordered {
					if ok || c != 0 {
						t.Errorf("%s %s Compare(%v, %v) = %d, %t, want unordered", name, f.name, tc.x, tc.y, c, ok)
					}
				} else if !ok || c != tc.want {
					t.Errorf("%s %s Compare(%v, %v) = %d, %t, want %d", name, f.name, tc.x, tc.y, c, ok, tc.want)
				}
				ordered := tc.want != unordered
				if got := im.equal(f, x, y); got != (ordered && tc.want == 0) {
					t.Errorf("%s %s Equal(%v, %v) = %t", name, f.name, tc.x, tc.y, got)
				}
				if got := im.less(f, x, y); got != (ordered && tc.want < 0) {
					t.Errorf("%s %s Less(%v, %v) = %t", name, f.name, tc.x, tc.y, got)
				}
				if got := im.lessEqual(f, x, y); got != (ordered && tc.want <= 0) {
					t.Errorf("%s %s LessEqual(%v, %v) = %t", name, f.name, tc.x, tc.y, got)
				}
				// The converse operands give the converse results.
				if ordered {
					if c2, ok := im.compare(f, y, x); !ok || c2 != -tc.want {
						t.Errorf("%s %s Compare(%v, %v) = %d, %t, want %d", name, f.name, tc.y, tc.x, c2, ok, -tc.want)
					}
				}
			}
			check(kernel{}, "kernel")
			check(reference{}, "reference")
		}
	}
}

// TestSignedZeroBits checks that -0 and +0 differ as bit patterns and
// equal as values (test/decimal.go testNegativeZero), and that the zeros
// are ordered by TotalOrder.
func TestSignedZeroBits(t *testing.T) {
	for _, f := range both {
		pz, nz := fin("0", 0).bits(f), nfin("0", 0).bits(f)
		if pz == nz {
			t.Fatalf("%s: -0 and +0 have the same bits", f.name)
		}
		k := kernel{}
		if !k.equal(f, pz, nz) || !k.equal(f, nz, pz) || k.less(f, nz, pz) || k.less(f, pz, nz) {
			t.Errorf("%s: -0 and +0 do not compare equal", f.name)
		}
		if !k.totalOrder(f, nz, pz) || k.totalOrder(f, pz, nz) {
			t.Errorf("%s: TotalOrder does not put -0 before +0", f.name)
		}
		if k.key(f, pz) != k.key(f, nz) {
			t.Errorf("%s: -0 and +0 have different keys", f.name)
		}
	}
}

// TestTotalOrderCases checks the order the contract states, in a list from
// the least opnd to the greatest. Each opnd precedes every later one
// and follows every earlier one; equal operands (the NaNs of one sign) are
// listed once.
func TestTotalOrderCases(t *testing.T) {
	ordered := []opnd{
		cNegNaN, cNegInf,
		nfin("1", 5), nfin("5", 0), nfin("1", 0), nfin("10", -1), nfin("100", -2), nfin("1", -5),
		nfin("0", 5), nfin("0", 0), nfin("0", -5),
		fin("0", -5), fin("0", 0), fin("0", 5),
		fin("1", -5), fin("100", -2), fin("10", -1), fin("1", 0), fin("5", 0), fin("1", 5),
		cInf, cNaN,
	}
	// Within a cohort the order is by exponent: for positive values the
	// smaller exponent first (100E-2, 10E-1, 1E0), for negative values the
	// larger first (-1E0, -10E-1, -100E-2).
	for _, f := range both {
		for i, a := range ordered {
			for j, b := range ordered {
				x, y := a.bits(f), b.bits(f)
				for _, im := range []impl{kernel{}, reference{}} {
					if got := im.totalOrder(f, x, y); got != (i <= j) {
						t.Errorf("%T %s TotalOrder(%v, %v) = %t, want %t", im, f.name, a, b, got, i <= j)
					}
				}
			}
		}
	}
}

// TestKeySign checks that Key carries the sign of non-zero finite values
// and infinities, which the strawman's hash key dropped, and not that of
// zeros and NaNs.
func TestKeySign(t *testing.T) {
	k := kernel{}
	for _, f := range both {
		key := func(o opnd) bid.Key { return k.key(f, o.bits(f)) }
		distinct := [][2]opnd{
			{fin("1", 0), nfin("1", 0)},
			{fin("5", 3), nfin("5", 3)},
			{fin("9999999999999999", 300), nfin("9999999999999999", 300)},
			{fin("1", -300), nfin("1", -300)},
			{cInf, cNegInf},
			{fin("1", 0), fin("2", 0)},
			{fin("1", 0), cInf},
			{fin("0", 0), cInf},
			{fin("1", 0), fin("0", 0)},
		}
		for _, p := range distinct {
			if key(p[0]) == key(p[1]) {
				t.Errorf("%s: Key(%v) == Key(%v)", f.name, p[0], p[1])
			}
		}
		same := [][2]opnd{
			{fin("0", 0), nfin("0", 0)},
			{fin("0", 300), nfin("0", -300)},
			{cNaN, cNegNaN},
			{cNaN, cSNaN},
			{fin("1", 0), fin("10", -1)},
			{nfin("1", 0), nfin("1000", -3)},
			{fin("100", 0), fin("1", 2)},
			{fin("1200", -2), fin("12", 0)},
			{fin("1", 300), fin("1000", 297)},
		}
		for _, p := range same {
			if key(p[0]) != key(p[1]) {
				t.Errorf("%s: Key(%v) != Key(%v): %+v, %+v", f.name, p[0], p[1], key(p[0]), key(p[1]))
			}
		}
		want := map[opnd]bid.Key{
			fin("1", 0):        {Coeff: bid.Uint128{Lo: 1}, Kind: bid.KindFinite},
			nfin("1", 0):       {Coeff: bid.Uint128{Lo: 1}, Kind: bid.KindFinite, Neg: true},
			fin("1200", -2):    {Coeff: bid.Uint128{Lo: 12}, Kind: bid.KindFinite},
			nfin("1000", 3):    {Coeff: bid.Uint128{Lo: 1}, Exp: 6, Kind: bid.KindFinite, Neg: true},
			fin("0", 7):        {Kind: bid.KindFinite},
			nfin("0", -7):      {Kind: bid.KindFinite},
			cInf:               {Kind: bid.KindInf},
			cNegInf:            {Kind: bid.KindInf, Neg: true},
			cNaN:               {Kind: bid.KindNaN},
			cNegNaN:            {Kind: bid.KindNaN},
			fin("12", 0):       {Coeff: bid.Uint128{Lo: 12}, Kind: bid.KindFinite},
			nfin("10", 0):      {Coeff: bid.Uint128{Lo: 1}, Exp: 1, Kind: bid.KindFinite, Neg: true},
			fin("1", 300):      {Coeff: bid.Uint128{Lo: 1}, Exp: 300, Kind: bid.KindFinite},
			fin("100000", -35): {Coeff: bid.Uint128{Lo: 1}, Exp: -30, Kind: bid.KindFinite},
		}
		for o, w := range want {
			if f == dec64 && o.e < -398 {
				continue
			}
			if got := key(o); got != w {
				t.Errorf("%s: Key(%v) = %+v, want %+v", f.name, o, got, w)
			}
		}
	}
}

// TestKeyNonCanonical checks that non-canonical encodings read as the
// value they decode to: a coefficient above the maximum is zero, whatever
// its exponent or sign.
func TestKeyNonCanonical(t *testing.T) {
	k := kernel{}
	zero := bid.Key{Kind: bid.KindFinite}
	// decimal64: the first form tops out at 2^53-1, below the maximum, so
	// only the second form, whose coefficient is 2^53 plus a 51-bit field,
	// is non-canonical: when the field reaches 10^16-2^53 = 992800745259008.
	const field64 = 992800745259008
	for _, x := range []uint64{
		3<<61 | 7<<51 | field64, 3<<61 | 7<<51 | (1<<51 - 1), 1<<63 | 3<<61 | 5<<51 | field64,
		3<<61 | 0<<51 | (1<<51 - 1), 3<<61 | 500<<51 | field64 + 1,
	} {
		if got := k.key(dec64, bits{Lo: x}); got != zero {
			t.Errorf("Key64(%016x) = %+v, want zero", x, got)
		}
		if !k.equal(dec64, bits{Lo: x}, fin("0", 0).bits(dec64)) {
			t.Errorf("Equal64(%016x, 0) is false", x)
		}
	}
	// decimal128: first form with a coefficient field above 10^34-1, and
	// the second form.
	maxHi, maxLo := uint64(bid.MaxCoeff128Hi), uint64(bid.MaxCoeff128Lo)
	for _, x := range []bits{
		{Hi: 7<<49 | maxHi, Lo: maxLo + 1},
		{Hi: 7<<49 | (1<<49 - 1), Lo: ^uint64(0)},
		{Hi: 1<<63 | 7<<49 | (maxHi + 1)},
		{Hi: 3<<61 | 5<<47, Lo: 12345},
		{Hi: 1<<63 | 3<<61 | 5<<47 | (1<<47 - 1), Lo: ^uint64(0)},
	} {
		if got := k.key(dec128, x); got != zero {
			t.Errorf("Key128(%016x%016x) = %+v, want zero", x.Hi, x.Lo, got)
		}
		if !k.equal(dec128, x, fin("0", 0).bits(dec128)) {
			t.Errorf("Equal128(%016x%016x, 0) is false", x.Hi, x.Lo)
		}
	}
	// Infinities and NaNs with stray payload bits are the same value as
	// the canonical ones.
	if k.key(dec64, bits{Lo: bid.Inf64 | 12345}) != k.key(dec64, bits{Lo: bid.Inf64}) {
		t.Error("Key64 of an infinity with stray bits differs from Key64(Inf)")
	}
	if k.key(dec64, bits{Lo: bid.SNaN64 | 12345}) != k.key(dec64, bits{Lo: bid.NaN64}) {
		t.Error("Key64 of a NaN with a payload differs from Key64(NaN)")
	}
	if k.key(dec128, bits{Hi: bid.SNaN128Hi | 1<<63, Lo: 99}) != k.key(dec128, bits{Hi: bid.NaN128Hi}) {
		t.Error("Key128 of a NaN with a payload differs from Key128(NaN)")
	}
}

// TestKeyWidenedAndLayout checks that a widened decimal64 has the same key
// in both widths and that Key is 24 bytes with no padding.
func TestKeyWidenedAndLayout(t *testing.T) {
	if sz := unsafe.Sizeof(bid.Key{}); sz != 24 {
		t.Errorf("Sizeof(Key) = %d, want 24", sz)
	}
	g := &gen{f: dec64, r: rand.New(rand.NewPCG(7, 8))}
	for i := range 20000 {
		g.regime = regime(i % int(numRegimes))
		x := g.dec()
		if got, want := bid.Key128(widen(x)), bid.Key64(x.Lo); got != want {
			t.Fatalf("Key128(Widen(%s)) = %+v, Key64 = %+v", display(dec64, x), got, want)
		}
		// Key is hashed as raw memory: every byte, the blank ones
		// included, must be determined by the value.
		key := bid.Key64(x.Lo)
		b := (*[unsafe.Sizeof(key)]byte)(unsafe.Pointer(&key))
		if b[22] != 0 || b[23] != 0 {
			t.Fatalf("Key64(%s) has a non-zero blank field: %x", display(dec64, x), b[:])
		}
	}
}

// TestComparisonsAllocate checks that nothing in the comparison API
// allocates.
func TestComparisonsAllocate(t *testing.T) {
	x64, y64 := fin("12", 3).bits(dec64).Lo, fin("1200", 1).bits(dec64).Lo
	x128, y128 := fin("12", 3).bits(dec128), fin("1200", 1).bits(dec128)
	var sink int
	var keySink bid.Key
	for name, fn := range map[string]func(){
		"Equal64":       func() { sink += b2i(bid.Equal64(x64, y64)) },
		"Less64":        func() { sink += b2i(bid.Less64(x64, y64)) },
		"LessEqual64":   func() { sink += b2i(bid.LessEqual64(x64, y64)) },
		"Compare64":     func() { c, _ := bid.Compare64(x64, y64); sink += c },
		"TotalOrder64":  func() { sink += b2i(bid.TotalOrder64(x64, y64)) },
		"Key64":         func() { keySink = bid.Key64(x64) },
		"Equal128":      func() { sink += b2i(bid.Equal128(x128, y128)) },
		"Less128":       func() { sink += b2i(bid.Less128(x128, y128)) },
		"LessEqual128":  func() { sink += b2i(bid.LessEqual128(x128, y128)) },
		"Compare128":    func() { c, _ := bid.Compare128(x128, y128); sink += c },
		"TotalOrder128": func() { sink += b2i(bid.TotalOrder128(x128, y128)) },
		"Key128":        func() { keySink = bid.Key128(x128) },
	} {
		if n := testing.AllocsPerRun(100, fn); n != 0 {
			t.Errorf("%s allocates %v times", name, n)
		}
	}
	_, _ = sink, keySink
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cohortMember returns an encoding of the value of v at a different
// exponent, shifted by j places if the coefficient and exponent range allow
// it (the nearest shift that does otherwise), or v itself when there is no
// other member. The value is unchanged; for zero only the exponent moves.
func cohortMember(f *format, v value, j int) value {
	w := value{kind: finite, neg: v.neg, coeff: new(big.Int).Set(v.coeff), exp: v.exp}
	if v.coeff.Sign() == 0 {
		w.exp = clamp(v.exp+j, f.minExp, f.maxExp)
		return w
	}
	for ; j != 0; j += sgn(-j) {
		switch {
		case j > 0 && digits(v.coeff)+j <= f.prec && v.exp-j >= f.minExp:
			w.coeff.Mul(v.coeff, pow10(j))
			w.exp = v.exp - j
			return w
		case j < 0 && trailingZeros(v.coeff) >= -j && v.exp-j <= f.maxExp:
			w.coeff.Quo(v.coeff, pow10(-j))
			w.exp = v.exp - j
			return w
		}
	}
	return w
}

func sgn(x int) int {
	switch {
	case x < 0:
		return -1
	case x > 0:
		return 1
	}
	return 0
}

// fuzzFormat returns the format a fuzz test selects with its wide argument.
func fuzzFormat(wide bool) *format {
	if wide {
		return dec128
	}
	return dec64
}

// rawBits builds an arbitrary bit pattern of f's width.
func rawBits(f *format, hi, lo uint64) bits {
	if f.width == 64 {
		return bits{Lo: lo}
	}
	return bits{Hi: hi, Lo: lo}
}

// FuzzKeyEqual checks that Key is equal whenever Equal is true, that the
// converse holds up to NaNs, and that the comparison agrees, for encodings
// of one value across quanta (including zeros of either sign and
// non-canonical encodings) and for arbitrary bit patterns. The seeds run
// under go test -short.
func FuzzKeyEqual(f *testing.F) {
	for _, wide := range []bool{false, true} {
		for _, neg := range []bool{false, true} {
			f.Add(wide, neg, uint64(0), uint64(1), 0, 0, uint64(0), uint64(0))
			f.Add(wide, neg, uint64(0), uint64(0), 5, 3, uint64(0), uint64(0))
			f.Add(wide, neg, uint64(0), uint64(1200), -2, 2, uint64(0), uint64(0))
			f.Add(wide, neg, uint64(0), uint64(1200), -2, -2, uint64(0), uint64(1)<<63)
			f.Add(wide, neg, uint64(1)<<20, uint64(1)<<63, 300, 7, uint64(1)<<63, uint64(0))
			f.Add(wide, neg, uint64(0), uint64(9999999999999999), -398, 5, uint64(0x7C<<56), uint64(0))
			f.Add(wide, neg, uint64(0x1ED09BEAD87C0), uint64(0x378D8E63FFFFFFFF), 6111, -9, uint64(0x78<<56), uint64(0))
		}
	}
	f.Fuzz(func(t *testing.T, wide, neg bool, chi, clo uint64, exp, shift int, zhi, zlo uint64) {
		fm := fuzzFormat(wide)
		k := kernel{}

		// One value in two quanta.
		c := new(big.Int).Lsh(new(big.Int).SetUint64(chi), 64)
		c.Or(c, new(big.Int).SetUint64(clo))
		c.Mod(c, new(big.Int).Add(fm.maxCoeff, bigOne))
		v := value{kind: finite, neg: neg, coeff: c, exp: clamp(exp, fm.minExp, fm.maxExp)}
		w := cohortMember(fm, v, shift%(fm.prec+2))
		if c.Sign() == 0 && shift&1 != 0 {
			w.neg = !w.neg // the other zero
		}
		x, y := fm.encode(v), fm.encode(w)
		if !k.equal(fm, x, y) || !k.equal(fm, y, x) {
			t.Fatalf("%s: Equal(%s, %s) is false for one value", fm.name, display(fm, x), display(fm, y))
		}
		if k.key(fm, x) != k.key(fm, y) {
			t.Fatalf("%s: Key(%s) = %+v, Key(%s) = %+v", fm.name, display(fm, x), k.key(fm, x), display(fm, y), k.key(fm, y))
		}
		if cmp, ok := k.compare(fm, x, y); cmp != 0 || !ok {
			t.Fatalf("%s: Compare(%s, %s) = %d, %t", fm.name, display(fm, x), display(fm, y), cmp, ok)
		}
		if got := k.key(fm, x); c.Sign() != 0 && got.Neg != neg || c.Sign() == 0 && got.Neg {
			t.Fatalf("%s: Key(%s) = %+v has the wrong sign", fm.name, display(fm, x), got)
		}

		// A non-canonical encoding of a zero: a coefficient field above
		// the maximum reads as zero at the same exponent.
		if c.Sign() == 0 {
			nc := nonCanonicalZero(fm, w.neg, w.exp, zlo)
			if !k.equal(fm, nc, y) || k.key(fm, nc) != k.key(fm, y) {
				t.Fatalf("%s: non-canonical zero %016x%016x differs from %s", fm.name, nc.Hi, nc.Lo, display(fm, y))
			}
		}

		// Any bit pattern against x.
		z := rawBits(fm, zhi, zlo)
		kz, kx := k.key(fm, z), k.key(fm, x)
		cmp, ok := k.compare(fm, x, z)
		eq := k.equal(fm, x, z)
		if eq != (ok && cmp == 0) {
			t.Fatalf("%s: Equal(%s, %016x%016x) = %t but Compare = %d, %t", fm.name, display(fm, x), z.Hi, z.Lo, eq, cmp, ok)
		}
		if eq && kz != kx {
			t.Fatalf("%s: Equal(%s, %016x%016x) but the keys differ: %+v, %+v", fm.name, display(fm, x), z.Hi, z.Lo, kx, kz)
		}
		if !eq && kz == kx {
			t.Fatalf("%s: the keys of %s and %016x%016x are equal but they are not Equal", fm.name, display(fm, x), z.Hi, z.Lo)
		}
		// Raw patterns against each other and against the widened value.
		z2 := fm.withSign(z, !fm.signbit(z))
		if k.equal(fm, z, z2) && k.key(fm, z) != k.key(fm, z2) {
			t.Fatalf("%s: Equal(%016x%016x, its negation) but the keys differ", fm.name, z.Hi, z.Lo)
		}
		if !wide {
			if bid.Key128(widen(x)) != bid.Key64(x.Lo) || bid.Key128(widen(z)) != bid.Key64(z.Lo) {
				t.Fatalf("Key128(Widen(x)) != Key64(x) for %s or %016x", display(fm, x), z.Lo)
			}
		}
	})
}

// nonCanonicalZero returns a non-canonical encoding of a zero with the
// given sign and exponent: a coefficient field above the maximum.
func nonCanonicalZero(f *format, neg bool, exp int, noise uint64) bits {
	biased := uint64(exp + f.bias)
	if f.width == 64 {
		// Second form with a field that takes the coefficient to 10^16
		// or more; the first form tops out below the maximum.
		const first = 992800745259008 // 10^16 - 2^53
		field := first + noise%(1<<51-first)
		return f.withSign(bits{Lo: 3<<61 | biased<<51 | field}, neg)
	}
	// Second form: always above the maximum, with any trailing bits.
	return f.withSign(bits{Hi: 3<<61 | biased<<47 | noise&(1<<47-1), Lo: noise}, neg)
}

// totalOrderSample returns a sample of encodings that is rich in the cases
// a total order must get right: members of cohorts, zeros of both signs,
// infinities, NaNs of both signs and kinds with payloads, non-canonical
// encodings, and arbitrary patterns.
func totalOrderSample(f *format, r *rand.Rand, n int) []bits {
	g := &gen{f: f, r: r}
	s := make([]bits, 0, n)
	for len(s) < n {
		switch r.IntN(6) {
		case 0:
			s = append(s, g.special())
		case 1:
			x, y := g.comparable()
			s = append(s, x, y)
		case 2:
			v := f.decode(g.dec())
			if v.kind == finite {
				for j := -3; j <= 3; j++ {
					s = append(s, f.encode(cohortMember(f, v, j)))
				}
			}
		case 3:
			s = append(s, nonCanonicalZero(f, r.IntN(2) == 0, g.exp(), r.Uint64()))
		case 4:
			s = append(s, rawBits(f, r.Uint64(), r.Uint64()))
		default:
			g.regime = regime(r.IntN(int(numRegimes)))
			s = append(s, g.dec())
		}
	}
	return s
}

// FuzzTotalOrder checks that TotalOrder is a total order on a random
// sample: total (one direction always holds), antisymmetric (both hold
// only for encodings that read as the same value in the same quantum and
// with the same sign, and then Compare is not "less"), transitive, and
// consistent with Compare. The seeds run under go test -short.
func FuzzTotalOrder(f *testing.F) {
	for seed := range uint64(4) {
		f.Add(false, seed)
		f.Add(true, seed)
	}
	f.Fuzz(func(t *testing.T, wide bool, seed uint64) {
		fm := fuzzFormat(wide)
		k := kernel{}
		r := rand.New(rand.NewPCG(seed, 0x70e1))
		s := totalOrderSample(fm, r, 48)
		le := make([][]bool, len(s))
		for i, x := range s {
			le[i] = make([]bool, len(s))
			for j, y := range s {
				le[i][j] = k.totalOrder(fm, x, y)
			}
		}
		for i, x := range s {
			if !le[i][i] {
				t.Fatalf("%s: TotalOrder(%s, itself) is false", fm.name, display(fm, x))
			}
			for j, y := range s {
				if !le[i][j] && !le[j][i] {
					t.Fatalf("%s: neither TotalOrder(%s, %s) nor its converse (%016x%016x, %016x%016x)", fm.name, display(fm, x), display(fm, y), x.Hi, x.Lo, y.Hi, y.Lo)
				}
				if le[i][j] && le[j][i] && !sameInOrder(fm, x, y) {
					t.Fatalf("%s: TotalOrder holds both ways for %s and %s (%016x%016x, %016x%016x)", fm.name, display(fm, x), display(fm, y), x.Hi, x.Lo, y.Hi, y.Lo)
				}
				if c, ok := k.compare(fm, x, y); ok && (c < 0 && !(le[i][j] && !le[j][i]) || c > 0 && !(le[j][i] && !le[i][j])) {
					t.Fatalf("%s: TotalOrder(%s, %s) disagrees with Compare %d", fm.name, display(fm, x), display(fm, y), c)
				}
				for l, z := range s {
					if le[i][j] && le[j][l] && !le[i][l] {
						t.Fatalf("%s: TotalOrder is not transitive: %s <= %s <= %s", fm.name, display(fm, x), display(fm, y), display(fm, z))
					}
				}
			}
		}
	})
}

// sameInOrder reports whether x and y are the same element of the total
// order: the same sign and, for finite values, the same value and
// exponent, or the same kind for infinities and NaNs.
func sameInOrder(f *format, x, y bits) bool {
	a, b := f.decode(x), f.decode(y)
	if a.kind != b.kind || a.neg != b.neg {
		return false
	}
	return a.kind != finite || a.exp == b.exp && a.coeff.Cmp(b.coeff) == 0
}

// TestComparisonsPropertiesLong is a bulk run of the properties of the
// fuzz tests for go test without -fuzz, with fresh samples per seed.
func TestComparisonsPropertiesLong(t *testing.T) {
	n := 20
	if testing.Short() {
		n = 4
	}
	for _, wide := range []bool{false, true} {
		fm := fuzzFormat(wide)
		t.Run(fm.name, func(t *testing.T) {
			for seed := range uint64(n) {
				r := rand.New(rand.NewPCG(seed, 99))
				s := totalOrderSample(fm, r, 200)
				k := kernel{}
				for i, x := range s {
					for _, y := range s[i:] {
						xy, yx := k.totalOrder(fm, x, y), k.totalOrder(fm, y, x)
						if !xy && !yx || xy && yx && !sameInOrder(fm, x, y) {
							t.Fatalf("%s: TotalOrder(%s, %s) = %t, converse %t", fm.name, display(fm, x), display(fm, y), xy, yx)
						}
						eq := k.equal(fm, x, y)
						bothNaN := fm.decode(x).kind == notANumber && fm.decode(y).kind == notANumber
						if (k.key(fm, x) == k.key(fm, y)) != (eq || bothNaN) {
							t.Fatalf("%s: Key equality of %s and %s disagrees with Equal %t", fm.name, display(fm, x), display(fm, y), eq)
						}
					}
				}
			}
		})
	}
}
