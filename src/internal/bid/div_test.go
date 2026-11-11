// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Directed tests of division beyond the conformance suite and the
// differential oracle: the reproducers of the strawman's four division
// faults, a generator of exact quotients that stresses the preferred
// exponent, fuzz targets against the reference model and the allocation
// and timing checks.

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

// divCase is one directed division: operands and the expected result in
// the character form, read and written by the reference model's parser
// and formatter, which do not call the kernel.
type divCase struct {
	f    *format
	x, y string
	mode bid.Mode
	want string
}

var divReproducers = []divCase{
	// Audit R3: the quotient must carry a round digit and an exact sticky
	// bit from the remainder whatever the digit counts; the strawman
	// rounded 1/7E+13 wrongly.
	{dec64, "1", "70000000000000", bid.ToNearestEven, "1.428571428571429E-14"},
	// Audit R2: the decimal128 quotient needs the 256-by-128 division to
	// reach all 34 digits.
	{dec128, "1", "12345678901234567890", bid.ToNearestEven, "8.100000072900000663471006037578055E-20"},
	// Audit R4: an exact quotient takes the exponent closest to Q(x) - Q(y),
	// here -2 - (-1) = -1, so 1.00 / 1.0 is 1.0, not 1 or 1.00.
	{dec64, "1.00", "1.0", bid.ToNearestEven, "1.0"},
	{dec128, "1.00", "1.0", bid.ToNearestEven, "1.0"},
	// Overflow through the preferred exponent: 1E+6138 / 1E-9 is exactly
	// 1E+6147, above the largest finite decimal128 value.
	{dec128, "1E+6138", "1E-9", bid.ToNearestEven, "Infinity"},
	{dec128, "1E+6138", "1E-9", bid.TowardZero, "9.999999999999999999999999999999999E+6144"},
	// The widest scaling: a one-digit dividend over a Precision-digit
	// divisor scales by 2×Precision places.
	{dec64, "1", "9999999999999999", bid.ToNearestEven, "1.000000000000000E-16"},
	{dec64, "9", "9999999999999999", bid.TowardPositive, "9.000000000000001E-16"},
	{dec128, "1", "9999999999999999999999999999999999", bid.ToNearestEven, "1.000000000000000000000000000000000E-34"},
	{dec128, "3", "9999999999999999999999999999999999", bid.TowardZero, "3.000000000000000000000000000000000E-34"},
	// The narrowest: a Precision-digit dividend over a one-digit divisor
	// scales by two places and the quotient has Precision+1 or +2 digits.
	{dec64, "9999999999999999", "7", bid.ToNearestEven, "1428571428571428"},
	{dec64, "9999999999999999", "1", bid.ToNearestEven, "9999999999999999"},
	{dec128, "9999999999999999999999999999999999", "7", bid.ToNearestEven, "1428571428571428571428571428571428"},
	// A quotient of 10^(Precision+1), the most trailing zeros the strip
	// can meet, with and without room to take the preferred exponent.
	{dec64, "1E+16", "1", bid.ToNearestEven, "1E+16"},
	{dec64, "1E+16", "1E-15", bid.ToNearestEven, "1E+31"},
	{dec64, "1000", "1E-13", bid.ToNearestEven, "1.000E+16"},
	// An exact value with more than Precision significant digits is
	// inexact: the remainder is zero but the strip finds no zeros.
	{dec64, "1", "1073741824", bid.ToNearestEven, "9.313225746154785E-10"},
	{dec64, "1", "1073741824", bid.TowardPositive, "9.313225746154786E-10"},
	// Ties: the dropped digits are exactly half and the remainder is zero.
	{dec64, "2000000000000001", "2", bid.ToNearestEven, "1000000000000000"},
	{dec64, "2000000000000001", "2", bid.ToNearestAway, "1000000000000001"},
	{dec64, "2000000000000003", "2", bid.ToNearestEven, "1000000000000002"},
	// Specials.
	{dec64, "0", "0", bid.ToNearestEven, "NaN"},
	{dec64, "Infinity", "-Infinity", bid.ToNearestEven, "NaN"},
	{dec64, "-1", "0", bid.ToNearestEven, "-Infinity"},
	{dec64, "Infinity", "-0", bid.ToNearestEven, "-Infinity"},
	{dec64, "-1", "Infinity", bid.TowardNegative, "-0E-398"},
	{dec64, "0E+5", "-1E-10", bid.ToNearestEven, "-0E+15"},
	{dec128, "0", "1E-6176", bid.ToNearestEven, "0E+6111"},
}

func TestDivReproducers(t *testing.T) {
	for _, c := range divReproducers {
		f := c.f
		x, st := f.parse(c.x, bid.ToNearestEven)
		if st != bid.Exact {
			t.Fatalf("%s: operand %q does not parse exactly", f.name, c.x)
		}
		y, st := f.parse(c.y, bid.ToNearestEven)
		if st != bid.Exact {
			t.Fatalf("%s: operand %q does not parse exactly", f.name, c.y)
		}
		got := kernel{}.div(f, x, y, c.mode)
		if s := display(f, got); s != c.want {
			t.Errorf("Div%d(%s, %s, %s) = %s, want %s", f.width, c.x, c.y, modeName(c.mode), s, c.want)
		}
		if ref := (reference{}).div(f, x, y, c.mode); ref != got {
			t.Errorf("Div%d(%s, %s, %s) = %s, reference %s", f.width, c.x, c.y, modeName(c.mode), display(f, got), display(f, ref))
		}
	}

	// The two reproducers whose claim is about the parts, not the text.
	x, _ := dec64.parse("1.00", bid.ToNearestEven)
	y, _ := dec64.parse("1.0", bid.ToNearestEven)
	if v := dec64.decode((kernel{}).div(dec64, x, y, bid.ToNearestEven)); v.coeff.Cmp(big.NewInt(10)) != 0 || v.exp != -1 {
		t.Errorf("Div64(1.00, 1.0) has coefficient %v and exponent %d, want 10 and -1", v.coeff, v.exp)
	}
	x, _ = dec128.parse("1", bid.ToNearestEven)
	y, _ = dec128.parse("12345678901234567890", bid.ToNearestEven)
	if v := dec128.decode((kernel{}).div(dec128, x, y, bid.ToNearestEven)); digits(v.coeff) != dec128.prec {
		t.Errorf("Div128(1, 12345678901234567890) has %d significant digits, want %d", digits(v.coeff), dec128.prec)
	}
}

// TestDivNearest drives the quotient that divLongW rounds to nearest by biasing
// its dividend with half the divisor, at the places where that can go
// wrong, in every mode against the reference:
//
//   - ties, a fraction of exactly a half, with odd and even quotients below
//     them: an odd dividend over a power of two whose exact quotient has
//     Precision+1 digits. The biased remainder is zero and the divisor even.
//   - a fraction one quarter either side of the tie: the same with
//     Precision+2 digits.
//   - an odd divisor whose biased remainder is zero, a fraction just above
//     the half that must round up and must not be taken for a tie: small odd
//     divisors, where one dividend in y is such a case.
//   - exact quotients, whose biased remainder is the half itself.
//   - the quotients nearest below a power of ten: 99…98 over 99…99.
//
// The test classifies each case itself, with math/big, and fails if a
// class is missing, so that the families keep reaching what they are for.
func TestDivNearest(t *testing.T) {
	var k kernel
	for _, f := range both {
		p := f.prec
		rng := rand.New(rand.NewPCG(*seedFlag, streamSeed("divnearest-"+f.name)))
		counts := map[string]int{}
		check := func(xc, yc *big.Int) {
			if xc.Sign() == 0 || yc.Sign() == 0 || xc.Cmp(f.maxCoeff) > 0 || yc.Cmp(f.maxCoeff) > 0 {
				return
			}
			// Classify: scale x so that the quotient has p digits.
			n, q, r := new(big.Int).Set(xc), new(big.Int), new(big.Int)
			for q.QuoRem(n, yc, r); digits(q) < p; q.QuoRem(n, yc, r) {
				n.Mul(n, big.NewInt(10))
			}
			if digits(q) == p {
				twice := new(big.Int).Lsh(r, 1)
				biased := new(big.Int).Add(n, new(big.Int).Rsh(yc, 1))
				biased.Mod(biased, yc)
				switch c := twice.Cmp(yc); {
				case r.Sign() == 0:
					counts["exact"]++
				case c == 0 && q.Bit(0) == 0:
					counts["tie, even below"]++
				case c == 0:
					counts["tie, odd below"]++
				case biased.Sign() == 0:
					counts["odd divisor, biased remainder zero"]++
				case c < 0:
					counts["below half"]++
				default:
					counts["above half"]++
				}
				if c := twice.Cmp(yc); c >= 0 && digits(new(big.Int).Add(q, big.NewInt(1))) > p {
					counts["carry"]++
				}
			}
			for _, e := range []int{0, -3} {
				for _, neg := range []bool{false, true} {
					x := f.encode(value{kind: finite, neg: neg, coeff: xc, exp: e})
					y := f.encode(value{kind: finite, coeff: yc, exp: 0})
					for _, m := range allModes {
						if got, want := k.div(f, x, y, m), f.div(x, y, m); got != want {
							t.Fatalf("%s div(%s, %s, %s) = %s, want %s", f.name, f.format(x), f.format(y), modeName(m), f.format(got), f.format(want))
						}
					}
				}
			}
		}
		// Odd dividends over powers of two: the exact quotient is x × 5^a
		// with a places, so its last digit is 5 and it has p+1 digits for
		// a tie and p+2 for a quarter either side.
		for a := 1; a <= 3*p; a++ {
			yc := new(big.Int).Lsh(big.NewInt(1), uint(a))
			five := new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(a)), nil)
			for _, extra := range []int{1, 2} {
				dx := p + extra - digits(five)
				for _, d := range []int{dx, dx + 1} {
					if d < 1 || d > p {
						continue
					}
					for range 40 {
						xc := randDigits(rng, d)
						xc.SetBit(xc, 0, 1)
						check(xc, yc)
						check(xc, new(big.Int).Mul(yc, big.NewInt(5))) // an even divisor that is not a power of two
					}
				}
			}
		}
		// Small divisors, odd and even, under every small dividend.
		for y := int64(2); y <= 200; y++ {
			for x := int64(1); x <= 120; x++ {
				check(big.NewInt(x), big.NewInt(y))
			}
		}
		// Random odd divisors of every length under dividends chosen for a
		// zero biased remainder where one exists nearby.
		for dy := 1; dy <= p; dy++ {
			for range 60 {
				yc := randDigits(rng, dy)
				yc.SetBit(yc, 0, 1)
				xc := randDigits(rng, 1+rng.IntN(p))
				check(xc, yc)
				check(new(big.Int).Mul(yc, big.NewInt(int64(1+rng.IntN(1000)))), yc) // exact
			}
		}
		// The quotients nearest below a power of ten: a dividend just
		// under the largest coefficient over that coefficient. None rounds
		// up out of the top digit, and no quotient of two coefficients
		// can: 99…9 and a half or more needs a dividend within y/2 of
		// y × 10^Precision after scaling by k places, a multiple of 10^k
		// that close only if 10^k is below y/2, which the scaling excludes.
		// The count stays to say so if that ever changes.
		for _, delta := range []int64{1, 2, 3, 7} {
			check(new(big.Int).Sub(f.maxCoeff, big.NewInt(delta)), f.maxCoeff)
		}
		if counts["carry"] != 0 {
			t.Errorf("%s: %d quotients carried out of the largest coefficient; none can", f.name, counts["carry"])
		}
		for _, class := range []string{"exact", "tie, even below", "tie, odd below", "odd divisor, biased remainder zero", "below half", "above half"} {
			if counts[class] == 0 {
				t.Errorf("%s: no case of class %q", f.name, class)
			}
		}
		t.Logf("%s: %v", f.name, counts)
	}
}

// randDigits returns a random integer of exactly d decimal digits.
func randDigits(r *rand.Rand, d int) *big.Int {
	n := big.NewInt(int64(1 + r.IntN(9)))
	for i := 1; i < d; i++ {
		n.Mul(n, big.NewInt(10))
		n.Add(n, big.NewInt(int64(r.IntN(10))))
	}
	return n
}

// TestDivExactQuotients stresses the preferred-exponent path with
// quotients known to be exact: q and y are drawn, x = q × y, and the
// exponents are drawn across the format. Every case is checked against the
// reference model and, independently of it, against the rule of "Exponent
// of a result": the result equals q × 10^(Q(x) - Q(y)) in value, and its
// exponent is the closest to Q(x) - Q(y) at which that value is
// representable (above it only when the coefficient has no trailing zero
// to give up or the exponent is MinExp, below it only when the coefficient
// already has Precision digits or the exponent is MaxExp).
func TestDivExactQuotients(t *testing.T) {
	n := 20000
	if testing.Short() {
		n = 2000
	}
	for _, f := range both {
		g := &gen{r: rand.New(rand.NewPCG(*seedFlag, 0x1d)), f: f}
		for i := range n {
			// dq + dy <= prec keeps x within the format; a few cases add
			// trailing zeros to x, as in 1.00 / 1.0, to move the preferred
			// exponent away from the quotient's natural one.
			dq := 1 + g.r.IntN(f.prec-1)
			dy := 1 + g.r.IntN(f.prec-dq)
			q := g.randDigits(dq)
			y := g.randDigits(dy)
			x := new(big.Int).Mul(q, y)
			if pad := f.prec - digits(x); pad > 0 && g.r.IntN(3) == 0 {
				p := pow10(1 + g.r.IntN(pad))
				x.Mul(x, p)
				q.Mul(q, p)
			}
			// Exponents: mostly moderate, sometimes anywhere in the format.
			var xe, ye int
			if i%4 == 0 {
				xe = f.minExp + g.r.IntN(f.maxExp-f.minExp+1)
				ye = f.minExp + g.r.IntN(f.maxExp-f.minExp+1)
			} else {
				xe = g.r.IntN(41) - 20
				ye = g.r.IntN(41) - 20
			}
			xb := f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: x, exp: xe})
			yb := f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: y, exp: ye})
			for m := bid.Mode(0); m < 5; m++ {
				got := kernel{}.div(f, xb, yb, m)
				if want := (reference{}).div(f, xb, yb, m); got != want {
					t.Errorf("Div%d(%s, %s, %s) = %s, reference %s", f.width, display(f, xb), display(f, yb), modeName(m), display(f, got), display(f, want))
					continue
				}
				v := f.decode(got)
				pref := xe - ye
				// Exact in value: coeff × 10^exp = q × 10^pref, unless the
				// value is outside the format, which the reference decides.
				if v.kind != finite {
					continue
				}
				lhs, rhs := new(big.Int).Set(v.coeff), new(big.Int).Set(q)
				if v.exp > pref {
					lhs.Mul(lhs, pow10(v.exp-pref))
				} else {
					rhs.Mul(rhs, pow10(pref-v.exp))
				}
				if lhs.Cmp(rhs) != 0 {
					// The value rounded, which is allowed only when no
					// encoding holds it: with q0 the coefficient stripped
					// of its tz trailing zeros, the exponents at which
					// q0 × 10^(pref+tz) fits are those within the format
					// from pref+tz down to where the coefficient reaches
					// Precision digits.
					tz := trailingZeros(q)
					q0 := new(big.Int).Quo(q, pow10(tz))
					top := min(pref+tz, f.maxExp)
					bottom := max(pref+tz-(f.prec-digits(q0)), f.minExp)
					if digits(q0) <= f.prec && bottom <= top {
						t.Errorf("Div%d(%s, %s, %s) = %s: not the exact quotient %v × 10^%d", f.width, display(f, xb), display(f, yb), modeName(m), display(f, got), q, pref)
					}
					continue
				}
				switch {
				case v.exp > pref && new(big.Int).Rem(v.coeff, bigTen).Sign() == 0 && v.exp != f.minExp:
					t.Errorf("Div%d(%s, %s, %s) = %s: exponent %d is above the preferred %d with a trailing zero to spare", f.width, display(f, xb), display(f, yb), modeName(m), display(f, got), v.exp, pref)
				case v.exp < pref && digits(v.coeff) < f.prec && v.exp != f.maxExp:
					t.Errorf("Div%d(%s, %s, %s) = %s: exponent %d is below the preferred %d with room in the coefficient", f.width, display(f, xb), display(f, yb), modeName(m), display(f, got), v.exp, pref)
				}
			}
		}
	}
}

// The fuzz targets compare the kernel with the reference model on raw bit
// patterns, canonical or not, under all five modes and one outside them.

func FuzzDiv64(f *testing.F) {
	seed := func(x, y string) {
		xb, _ := dec64.parse(x, bid.ToNearestEven)
		yb, _ := dec64.parse(y, bid.ToNearestEven)
		f.Add(xb.Lo, yb.Lo)
	}
	seed("1", "70000000000000")
	seed("1.00", "1.0")
	seed("1", "3")
	seed("2000000000000001", "2")
	seed("1E+300", "1E-300")
	seed("1E-398", "3")
	seed("0", "0")
	seed("Infinity", "1")
	f.Add(uint64(0x6c77ffffffffffff), uint64(0x6c7386f26fc10000))
	f.Fuzz(func(t *testing.T, x, y uint64) {
		for m := bid.Mode(0); m < 6; m++ {
			got := bid.Div64(x, y, m)
			want := reference{}.div(dec64, lo(x), lo(y), m)
			if got != want.Lo {
				t.Errorf("Div64(%s, %s, %s) = %s; reference %s", display(dec64, lo(x)), display(dec64, lo(y)), modeName(m), display(dec64, lo(got)), display(dec64, want))
			}
		}
	})
}

func FuzzDiv128(f *testing.F) {
	seed := func(x, y string) {
		xb, _ := dec128.parse(x, bid.ToNearestEven)
		yb, _ := dec128.parse(y, bid.ToNearestEven)
		f.Add(xb.Hi, xb.Lo, yb.Hi, yb.Lo)
	}
	seed("1", "12345678901234567890")
	seed("1.00", "1.0")
	seed("1E+6138", "1E-9")
	seed("1", "3")
	seed("2000000000000000000000000000000001", "2")
	seed("1E-6176", "3")
	seed("0", "0")
	seed("Infinity", "-Infinity")
	f.Add(uint64(0x3041ed09bead87c0), uint64(0x378d8e6400000000), uint64(0x3041ffffffffffff), uint64(0xffffffffffffffff))
	f.Fuzz(func(t *testing.T, xh, xl, yh, yl uint64) {
		x, y := bid.Uint128{Hi: xh, Lo: xl}, bid.Uint128{Hi: yh, Lo: yl}
		for m := bid.Mode(0); m < 6; m++ {
			got := bid.Div128(x, y, m)
			want := reference{}.div(dec128, x, y, m)
			if got != want {
				t.Errorf("Div128(%s, %s, %s) = %s; reference %s", display(dec128, x), display(dec128, y), modeName(m), display(dec128, got), display(dec128, want))
			}
		}
	})
}

// TestDivWords drives div128's own paths, for a divisor of one word and of
// two, against the reference in every mode (the directed modes take the
// general path, which the same operands then test): every pair of digit
// counts, so that each coefficient is of one word and of two, the
// dividend's scale crosses the last power of ten of each table and the
// leading ratio falls on both sides of one;
// exact quotients, whose zeros are stripped; divisors that are powers of
// ten; ties and their neighbours, under small even divisors; the largest
// words; and exponents that put the quotient at and beyond both ends of
// the range, where the path must refuse rather than encode.
func TestDivWords(t *testing.T) {
	var k kernel
	f := dec128
	rng := rand.New(rand.NewPCG(*seedFlag, streamSeed("divwords")))
	n := 0
	check := func(xc, yc *big.Int, xe, ye int) {
		if xc.Cmp(f.maxCoeff) > 0 || yc.Cmp(f.maxCoeff) > 0 {
			return
		}
		for _, neg := range []bool{false, true} {
			x := f.encode(value{kind: finite, neg: neg, coeff: xc, exp: xe})
			y := f.encode(value{kind: finite, coeff: yc, exp: ye})
			for _, m := range allModes {
				n++
				if got, want := k.div(f, x, y, m), f.div(x, y, m); got != want {
					t.Fatalf("div(%s, %s, %s) = %s, want %s", f.format(x), f.format(y), modeName(m), f.format(got), f.format(want))
				}
			}
		}
	}
	exps := func() (int, int) {
		switch rng.IntN(8) {
		case 0: // the quotient near the smallest exponent
			return f.minExp + rng.IntN(60), rng.IntN(40)
		case 1: // and near the largest
			return f.maxExp - rng.IntN(60), -rng.IntN(40)
		}
		return rng.IntN(41) - 20, rng.IntN(41) - 20
	}
	rounds := 40
	if *longTest {
		rounds = 2000
	}
	for dx := 1; dx <= f.prec; dx++ {
		for dy := 1; dy <= f.prec; dy++ {
			for range rounds {
				xc, yc := randDigits(rng, dx), randDigits(rng, dy)
				xe, ye := exps()
				check(xc, yc, xe, ye)
				// An exact quotient: the dividend is a multiple.
				check(new(big.Int).Mul(yc, randDigits(rng, 1+rng.IntN(dx))), yc, xe, ye)
				// A power of ten below and the words either side of it.
				p10 := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(dy-1)), nil)
				check(xc, p10, xe, ye)
				check(xc, new(big.Int).Add(p10, big.NewInt(1)), xe, ye)
				if dy > 1 {
					check(xc, new(big.Int).Sub(p10, big.NewInt(1)), xe, ye)
				}
			}
		}
	}
	// Ties and the fractions next to them: odd dividends over powers of
	// two and of two times five, and over small divisors of each parity.
	for a := 1; a <= 112; a++ {
		yc := new(big.Int).Lsh(big.NewInt(1), uint(a))
		for range rounds {
			xc := new(big.Int).SetUint64(rng.Uint64() | 1)
			check(xc, yc, 0, 0)
			check(new(big.Int).SetUint64(rng.Uint64()>>uint(rng.IntN(64))|1), yc, 0, 0)
			// Two words: an odd dividend of every length up to the
			// precision, whose quotient ends in a five where a tie is.
			long := randDigits(rng, 20+rng.IntN(f.prec-19))
			check(long.SetBit(long, 0, 1), yc, 0, 0)
		}
	}
	for y := uint64(1); y <= 2000; y++ {
		yc := new(big.Int).SetUint64(y)
		for range max(rounds/20, 2) {
			check(new(big.Int).SetUint64(rng.Uint64()>>uint(rng.IntN(64))), yc, 0, 0)
			check(randDigits(rng, 20+rng.IntN(f.prec-19)), yc, 0, 0)
		}
	}
	// The largest coefficient and its neighbours, over divisors of each
	// length: the leading ratio at its extremes with no scale to spare.
	for _, d := range []int64{0, 1, 2} {
		xc := new(big.Int).Sub(f.maxCoeff, big.NewInt(d))
		for dy := 1; dy <= f.prec; dy++ {
			for range max(rounds/10, 2) {
				check(xc, randDigits(rng, dy), 0, 0)
				check(randDigits(rng, dy), xc, 0, 0)
			}
			nines := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(dy)), nil)
			check(xc, nines.Sub(nines, big.NewInt(1)), 0, 0)
		}
	}
	// The largest words, and divisors at and around each power of two.
	for _, x := range []uint64{^uint64(0), ^uint64(0) - 1, 1 << 63, 1<<63 - 1, 1<<63 + 1, 9999999999999999999, 10000000000000000000} {
		for _, y := range []uint64{^uint64(0), ^uint64(0) - 1, 1 << 63, 1<<63 - 1, 1<<63 + 1, 1 << 32, 1<<32 - 1, 1<<32 + 1, 3, 7, 9999999999999999999, 10000000000000000000} {
			check(new(big.Int).SetUint64(x), new(big.Int).SetUint64(y), 0, 0)
			check(new(big.Int).SetUint64(y), new(big.Int).SetUint64(x), 0, 0)
		}
	}
	for s := 1; s < 64; s++ {
		for _, d := range []uint64{0, 1} {
			yc := new(big.Int).SetUint64(uint64(1)<<s - 1 + d<<1)
			for range max(rounds/10, 2) {
				check(new(big.Int).SetUint64(rng.Uint64()), yc, 0, 0)
			}
		}
	}
	// Ties under a divisor of two words: a small even number times a power
	// of ten, under an odd dividend of the full precision, whose quotient
	// by two then has one digit more than the precision, a five; and the
	// same under the divisors that put a quarter or an eighth there.
	for j := 19; j < f.prec; j++ {
		p10 := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(j)), nil)
		for _, d := range []int64{2, 4, 8, 5, 6, 16, 25, 50} {
			yc := new(big.Int).Mul(p10, big.NewInt(d))
			for range max(rounds/4, 4) {
				xc := randDigits(rng, f.prec-rng.IntN(3))
				check(xc.SetBit(xc, 0, 1), yc, 0, 0)
			}
		}
	}
	// An odd divisor whose biased remainder is zero: a fraction just above
	// the half, which rounds up and is no tie. For a divisor y prime to
	// ten and a scale of k places, the quotients Q with
	// (2Q + 1) y = -1 modulo 2 × 10^k are the ones: the dividend
	// (Q y + (y+1)/2) / 10^k is then whole, and it is tried when it has
	// no more digits than the precision.
	found := 0
	for dy := 1; dy <= f.prec; dy++ {
		for range max(rounds, 40) {
			yc := randDigits(rng, dy)
			yc.SetBit(yc, 0, 1)
			if new(big.Int).Mod(yc, big.NewInt(5)).Sign() == 0 {
				continue
			}
			k := max(dy-1, 1) + rng.IntN(3)
			m := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(k)), nil)
			m2 := new(big.Int).Lsh(m, 1)
			inv := new(big.Int).ModInverse(yc, m2)
			q := new(big.Int).Sub(m2, inv) // 2Q + 1 modulo 2 × 10^k
			q.Rsh(q, 1)                    // Q modulo 10^k
			// The Q of the full precision in that class, at random.
			top := randDigits(rng, f.prec)
			top.Sub(top, new(big.Int).Mod(top, m)).Add(top, q)
			if digits(top) != f.prec {
				continue
			}
			xc := new(big.Int).Mul(top, yc)
			xc.Add(xc, new(big.Int).Rsh(new(big.Int).Add(yc, big.NewInt(1)), 1))
			if new(big.Int).Mod(xc, m).Sign() != 0 {
				t.Fatalf("construction: %v not a multiple of 10^%d", xc, k)
			}
			xc.Quo(xc, m)
			if xc.Cmp(f.maxCoeff) <= 0 {
				found++
				check(xc, yc, 0, 0)
			}
		}
	}
	if found < 100 {
		t.Fatalf("only %d odd divisors with a biased remainder of zero", found)
	}
	// Zeros, which the word path hands on, with every sort of word beside
	// them.
	for _, v := range []uint64{1, 2, 10, 7890, 1 << 32, ^uint64(0)} {
		for _, e := range []int{-30, 0, 30} {
			check(new(big.Int), new(big.Int).SetUint64(v), e, 0)
			check(new(big.Int).SetUint64(v), new(big.Int), e, 0)
		}
	}
	check(new(big.Int), new(big.Int), 0, 0)
	if n == 0 {
		t.Fatal("no division checked")
	}
}

// TestPow10Word checks the padded copy of the word powers of ten that the
// word path of division indexes without a bound test.
func TestPow10Word(t *testing.T) {
	p := big.NewInt(1)
	for i, v := range bid.Pow10Word {
		switch {
		case p.IsUint64() && v != p.Uint64():
			t.Errorf("pow10word[%d] = %d, want %v", i, v, p)
		case !p.IsUint64() && v != 0:
			t.Errorf("pow10word[%d] = %d, want 0", i, v)
		}
		p.Mul(p, big.NewInt(10))
	}
}

// TestDivZeroOperands checks the quotients that div128 decides without a
// division, a zero dividend over a divisor of one word and a dividend
// over zero, and the neighbours it leaves to divLong128: 0 / 0, a
// dividend of two words, a zero whose exponent takes the quotient's
// outside the format, and a dividend whose coefficient is not canonical,
// which reads as zero and so over zero is invalid. The reference decides,
// in every mode and for every pair of signs.
func TestDivZeroOperands(t *testing.T) {
	var k kernel
	f := dec128
	fin := func(c *big.Int, exp int) bits {
		return f.encode(value{kind: finite, coeff: c, exp: exp})
	}
	// A first-form encoding with a coefficient above the largest: 10^34.
	nonCanonical := toUint128(pow10(f.prec))
	nonCanonical.Hi |= uint64(bid.Bias128) << bid.ExpShift128
	xs := []bits{
		fin(new(big.Int), 0), fin(new(big.Int), -2), fin(new(big.Int), f.maxExp), fin(new(big.Int), f.minExp),
		fin(big.NewInt(1), 0), fin(big.NewInt(123456), -2), fin(new(big.Int).SetUint64(1<<64-1), 7),
		fin(f.maxCoeff, 0), fin(pow10(20), f.minExp), nonCanonical,
	}
	ys := []bits{
		fin(new(big.Int), 0), fin(new(big.Int), -2), fin(new(big.Int), f.maxExp), fin(new(big.Int), f.minExp),
		fin(big.NewInt(3), 0), fin(big.NewInt(789), -2), fin(big.NewInt(1), f.minExp), fin(big.NewInt(1), f.maxExp),
		fin(f.maxCoeff, 0),
	}
	neg := bits{Hi: 1 << 63}
	for _, x := range xs {
		for _, y := range ys {
			for _, sx := range []bits{{}, neg} {
				for _, sy := range []bits{{}, neg} {
					x, y := bits{Hi: x.Hi | sx.Hi, Lo: x.Lo}, bits{Hi: y.Hi | sy.Hi, Lo: y.Lo}
					for _, m := range allModes {
						if got, want := k.div(f, x, y, m), f.div(x, y, m); got != want {
							t.Errorf("div(%s, %s, %s) = %s, want %s", f.format(x), f.format(y), modeName(m), f.format(got), f.format(want))
						}
					}
				}
			}
		}
	}
}
