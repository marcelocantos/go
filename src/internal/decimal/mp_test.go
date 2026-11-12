// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal_test

import (
	"internal/bid"
	. "internal/decimal"
	"math/big"
	"math/rand/v2"
	"testing"
)

// These tests check the multiple-precision arithmetic under the
// elementary functions against math/big: that each operation is within
// the error its comment states, and that the logarithm and exponential
// built from them are within the bounds the rounding loop relies on.
// The functions themselves are checked in math/decimal.

const bigPrec = 4000

func bigf() *big.Float { return new(big.Float).SetPrec(bigPrec) }

// toBig returns x exactly.
func toBig(x Mpf) *big.Float {
	m, exp, neg := x.Parts()
	v := new(big.Int)
	for i := len(m) - 1; i >= 0; i-- {
		v.Lsh(v, 64).Or(v, new(big.Int).SetUint64(m[i]))
	}
	f := bigf().SetInt(v)
	f.SetMantExp(f, exp)
	if neg {
		f.Neg(f)
	}
	return f
}

// ulps returns |got - want| in units of the last of n words of want.
func ulps(n int, got Mpf, want *big.Float) float64 {
	if want.Sign() == 0 {
		if m, _, _ := got.Parts(); m == nil {
			return 0
		}
		return 1e300
	}
	d := bigf().Sub(toBig(got), want)
	d.SetMantExp(d, 64*n-want.MantExp(nil))
	u, _ := d.Abs(d).Float64()
	return u
}

func randMpf(r *rand.Rand, n int) Mpf {
	m := make([]uint64, n)
	for i := range m {
		switch r.IntN(8) {
		case 0:
			m[i] = 0
		case 1:
			m[i] = ^uint64(0)
		default:
			m[i] = r.Uint64()
		}
	}
	m[n-1] |= 1 << 63
	return MakeMpf(m, r.IntN(801)-400-64*n, r.IntN(2) == 0)
}

func TestMPArithmetic(t *testing.T) {
	r := rand.New(rand.NewPCG(2, 3))
	check := func(what string, n int, got Mpf, want *big.Float, max float64) {
		t.Helper()
		if m, _, _ := got.Parts(); m != nil && (len(m) != n || m[n-1]>>63 == 0) {
			t.Fatalf("%s at %d words: result not normalized", what, n)
		}
		if u := ulps(n, got, want); u > max {
			t.Fatalf("%s at %d words: off by %g units in the last place, want at most %g", what, n, u, max)
		}
	}
	for i := 0; i < 20000; i++ {
		n := 1 + r.IntN(9)
		x, y := randMpf(r, 1+r.IntN(n+1)), randMpf(r, 1+r.IntN(n+1))
		if i%3 == 0 { // operands close together, so that sums cancel
			m, exp, _ := x.Parts()
			m = append([]uint64(nil), m...)
			m[0] ^= r.Uint64() >> uint(r.IntN(64))
			y = MakeMpf(m, exp, r.IntN(2) == 0)
		}
		bx, by := toBig(x), toBig(y)
		check("mul", n, MpMul(n, x, y), bigf().Mul(bx, by), 1)
		check("add", n, MpAdd(n, x, y), bigf().Add(bx, by), 1)
		check("sub", n, MpSub(n, x, y), bigf().Sub(bx, by), 1)
		check("div", n, MpDiv(n, x, y), bigf().Quo(bx, by), 1)
		w := r.Uint64()>>uint(r.IntN(64)) | 1
		check("mulWord", n, MpMulWord(n, x, w), bigf().Mul(bx, bigf().SetUint64(w)), 1)
		check("divWord", n, MpDivWord(n, x, w), bigf().Quo(bx, bigf().SetUint64(w)), 1)
		if i%10 == 0 {
			a := MakeMpfAbs(x)
			check("sqrt", n, MpSqrt(n, a), bigf().Sqrt(toBig(a)), 4)
			e := r.IntN(14001) - 7000
			p := bigf().SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(max(e, -e))), nil))
			if e < 0 {
				p.Quo(bigf().SetInt64(1), p)
			}
			check("pow10", n, MpPow10(n, e), p, 2)
			q := r.IntN(121) - 60
			p10 := bigf().SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(max(q, -q))), nil))
			scaled := bigf().Quo(bx, p10)
			if q < 0 {
				scaled = bigf().Mul(bx, p10)
			}
			check("scale10", n, MpScale10(n, x, q), scaled, 4)
			// An integer power of a value near 1, exactly: its error
			// is within eight units for each unit of the count.
			k := int64(r.IntN(4001) - 2000)
			base := MpAdd(n, MakeMpf([]uint64{1 << 63}, -63, false), MakeMpf([]uint64{r.Uint64() | 1<<63}, -64-r.IntN(40), r.IntN(2) == 0))
			if got, ok := MpPowInt(n, base, k); ok {
				want := bigf().SetInt64(1)
				bb := toBig(base)
				for j := max(k, -k); j > 0; j-- {
					want.Mul(want, bb)
				}
				if k < 0 {
					want.Quo(bigf().SetInt64(1), want)
				}
				check("powInt", n, got, want, float64(8*max(k, -k)+8))
			}
		}
	}
}

// MakeMpfAbs returns |x|.
func MakeMpfAbs(x Mpf) Mpf {
	m, exp, _ := x.Parts()
	return MakeMpf(m, exp, false)
}

// bigAtanh2 returns 2 atanh(z) by its series.
func bigAtanh2(z *big.Float) *big.Float {
	z2 := bigf().Mul(z, z)
	sum, term := bigf(), bigf().Set(z)
	for i := int64(1); term.Sign() != 0 && term.MantExp(nil)-z.MantExp(nil) > -bigPrec; i += 2 {
		sum.Add(sum, bigf().Quo(term, bigf().SetInt64(i)))
		term.Mul(term, z2)
	}
	return sum.Mul(sum, bigf().SetInt64(2))
}

// bigLog returns ln x: square roots down to near 1, then the series.
func bigLog(x *big.Float) *big.Float {
	one := bigf().SetInt64(1)
	y, k := bigf().Set(x), 0
	for d := bigf().Sub(y, one); d.Sign() != 0 && d.MantExp(nil) > -40; d.Sub(y, one) {
		y.Sqrt(y)
		k++
	}
	v := bigAtanh2(bigf().Quo(bigf().Sub(y, one), bigf().Add(y, one)))
	return v.SetMantExp(v, k)
}

// bigExp returns e^t for |t| below 2^16: halved to 2^-120 at most, summed
// and squared back.
func bigExp(t *big.Float) *big.Float {
	m := 120 + max(t.MantExp(nil), 0)
	r := bigf().SetMantExp(t, -m)
	sum, term := bigf().SetInt64(1), bigf().SetInt64(1)
	for i := int64(1); ; i++ {
		term.Mul(term, r)
		term.Quo(term, bigf().SetInt64(i))
		if term.Sign() == 0 || term.MantExp(nil) < -bigPrec {
			break
		}
		sum.Add(sum, term)
	}
	for ; m > 0; m-- {
		sum.Mul(sum, sum)
	}
	return sum
}

func TestLnConstants(t *testing.T) {
	ln2 := bigAtanh2(bigf().Quo(bigf().SetInt64(1), bigf().SetInt64(3)))
	ln10 := bigLog(bigf().SetInt64(10))
	for n := 1; n <= LnTableWords+3; n++ {
		// The table's words are the truncation of the constant, and
		// beyond the table the series is within its stated bound.
		limit := 1.0
		if n > LnTableWords {
			limit = 4096
		}
		if u := ulps(n, MpLn2(n), ln2); u > limit {
			t.Errorf("ln 2 at %d words: off by %g units", n, u)
		}
		if u := ulps(n, MpLn10(n), ln10); u > limit {
			t.Errorf("ln 10 at %d words: off by %g units", n, u)
		}
		if u := ulps(n, MpLn2Series(n), ln2); u > 4096 {
			t.Errorf("ln 2 by series at %d words: off by %g units", n, u)
		}
		if u := ulps(n, MpLn10Series(n), ln10); u > 4096 {
			t.Errorf("ln 10 by series at %d words: off by %g units", n, u)
		}
	}
}

// TestMPFunctions checks the bounds the rounding loop of the elementary
// functions assumes: a logarithm within 2^-(64n-14) of the exact one, in
// relative terms, and an exponential within 2^-(64n-30) for an argument
// that is itself within 2^-(64n-14). In units of the last place those
// are 2^13 and 2^29.
func TestMPFunctions(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 8))
	var worstLn, worstExp float64
	for i := 0; i < 3000; i++ {
		n := []int{3, 4, 4, 8, 16}[r.IntN(5)]
		// A coefficient of up to 38 digits and an exponent across the
		// range, and one in three near 1 from either side.
		digits := 1 + r.IntN(38)
		c := new(big.Int)
		for j := 0; j < digits; j++ {
			c.Mul(c, big.NewInt(10)).Add(c, big.NewInt(int64(r.IntN(10))))
		}
		if c.Sign() == 0 {
			c.SetInt64(7)
		}
		e := r.IntN(12401) - 6200
		if i%3 == 0 {
			k := r.IntN(37)
			c = new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(k)), nil)
			c.Add(c, big.NewInt(int64(r.IntN(19)-9)))
			if k == 0 && c.Cmp(big.NewInt(1)) <= 0 {
				c.SetInt64(3)
			}
			e = -k
		}
		u := bid.Uint128{Hi: new(big.Int).Rsh(c, 64).Uint64(), Lo: c.Uint64()}
		x := bigf().SetInt(c)
		p := bigf().SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(max(e, -e))), nil))
		if e < 0 {
			x.Quo(x, p)
		} else {
			x.Mul(x, p)
		}
		if x.Cmp(bigf().SetInt64(1)) == 0 {
			continue
		}
		want := bigLog(x)
		got := MpLn(n, u, e)
		ul := ulps(n, got, want)
		worstLn = max(worstLn, ul)
		if ul > 1<<13 {
			t.Fatalf("ln(%vE%d) at %d words: off by %g units, want at most 2^13", c, e, n, ul)
		}

		// The exponential of a multiple of that logarithm, kept in the
		// range the functions use it in.
		k := bigf().SetInt64(int64(r.IntN(2001) - 1000))
		if r.IntN(2) == 0 {
			k.Quo(k, bigf().SetInt64(int64(1+r.IntN(1000))))
		}
		tb := bigf().Mul(want, k)
		if tb.Sign() == 0 || tb.MantExp(nil) > 15 {
			continue
		}
		// Round the argument to n words, as a caller's would be.
		mant := new(big.Float).SetPrec(uint(64 * n)).SetMode(big.ToZero)
		mant.Set(tb)
		mi, _ := new(big.Float).SetMantExp(mant, 64*n-mant.MantExp(nil)).Int(nil)
		words := make([]uint64, n)
		abs := new(big.Int).Abs(mi)
		for j := range words {
			words[j] = new(big.Int).Rsh(abs, uint(64*j)).Uint64()
		}
		arg := MakeMpf(words, mant.MantExp(nil)-64*n, tb.Sign() < 0)
		v, ok := MpExp(n, arg)
		if !ok {
			continue
		}
		ue := ulps(n, v, bigExp(toBig(arg)))
		worstExp = max(worstExp, ue)
		if ue > 1<<29 {
			t.Fatalf("exp at %d words: off by %g units, want at most 2^29", n, ue)
		}
	}
	t.Logf("largest errors seen, in units of the last place: ln %g, exp %g", worstLn, worstExp)
}

func TestIntegerHelpers(t *testing.T) {
	r := rand.New(rand.NewPCG(13, 21))
	big128 := func(u bid.Uint128) *big.Int {
		v := new(big.Int).SetUint64(u.Hi)
		return v.Lsh(v, 64).Or(v, new(big.Int).SetUint64(u.Lo))
	}
	ten := big.NewInt(10)
	for i := 0; i < 20000; i++ {
		u := bid.Uint128{Hi: r.Uint64() >> uint(r.IntN(65)), Lo: r.Uint64() >> uint(r.IntN(64))}
		if u.Hi|u.Lo == 0 {
			u.Lo = 1
		}
		if r.IntN(3) == 0 { // trailing zeros
			z := new(big.Int).Exp(ten, big.NewInt(int64(1+r.IntN(20))), nil)
			v := new(big.Int).Mul(new(big.Int).SetUint64(u.Lo>>32|1), z)
			u = bid.Uint128{Hi: new(big.Int).Rsh(v, 64).Uint64(), Lo: v.Uint64()}
		}
		b := big128(u)
		if got, want := DigitsU(u), len(b.String()); got != want {
			t.Fatalf("digitsU(%v) = %d, want %d", b, got, want)
		}
		s, z := StripU(u)
		want := new(big.Int).Set(b)
		wz := 0
		for new(big.Int).Mod(want, ten).Sign() == 0 {
			want.Quo(want, ten)
			wz++
		}
		if big128(s).Cmp(want) != 0 || z != wz {
			t.Fatalf("stripU(%v) = %v, %d; want %v, %d", b, big128(s), z, want, wz)
		}
		k := r.IntN(39)
		q, exact := DivPow10U(u, k)
		wq, wr := new(big.Int).QuoRem(b, new(big.Int).Exp(ten, big.NewInt(int64(k)), nil), new(big.Int))
		if big128(q).Cmp(wq) != 0 || exact != (wr.Sign() == 0) {
			t.Fatalf("divPow10U(%v, %d) = %v, %v; want %v, %v", b, k, big128(q), exact, wq, wr.Sign() == 0)
		}
		// Powers and roots of a small base.
		base := bid.Uint128{Lo: 1 + r.Uint64()>>uint(40+r.IntN(24))}
		n := uint(1 + r.IntN(140))
		wp := new(big.Int).Exp(big128(base), big.NewInt(int64(n)), nil)
		p, ok := PowU(base, n)
		if ok != (wp.BitLen() <= 128) || ok && big128(p).Cmp(wp) != 0 {
			t.Fatalf("powU(%v, %d) = %v, %v; want %v", base.Lo, n, big128(p), ok, wp)
		}
		if ok {
			if root, isRoot := RootU(p, n); !isRoot || base.Lo != 1 && root != base {
				t.Fatalf("rootU(%v, %d) = %v, %v; want %v", wp, n, big128(root), isRoot, base.Lo)
			}
			if p.Lo != ^uint64(0) && n > 1 && base.Lo > 1 {
				p.Lo++
				if root, isRoot := RootU(p, n); isRoot {
					t.Fatalf("rootU(%v + 1, %d) = %v, true", wp, n, big128(root))
				}
			}
		}
	}
}
