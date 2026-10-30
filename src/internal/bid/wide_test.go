// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import (
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"math/rand/v2"
	"testing"
)

// Tests of the 128- and 256-bit arithmetic of u128.go and u256.go against
// math/big. Each primitive has a fuzz target. The seed corpus of each runs
// under go test, including -short, so the edge cases (carries across the
// word boundary, shifts at 0, 64 and 128, the quotient-estimate cases of
// division) are checked on every run; fuzzing only adds to them.

var (
	bigOne    = big.NewInt(1)
	bigTwo64  = new(big.Int).Lsh(bigOne, 64)
	bigTwo128 = new(big.Int).Lsh(bigOne, 128)
	bigTwo256 = new(big.Int).Lsh(bigOne, 256)
)

func bigPow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

func bigOf256(a Uint256) *big.Int {
	n := bigOf(a.Hi)
	n.Lsh(n, 128)
	return n.Or(n, bigOf(a.Lo))
}

// from128 converts x, which must be in [0, 2^128), to a Uint128.
func from128(x *big.Int) Uint128 {
	if x.Sign() < 0 || x.Cmp(bigTwo128) >= 0 {
		panic(fmt.Sprintf("%v does not fit 128 bits", x))
	}
	m := new(big.Int).SetUint64(math.MaxUint64)
	return Uint128{
		Hi: new(big.Int).And(new(big.Int).Rsh(x, 64), m).Uint64(),
		Lo: new(big.Int).And(x, m).Uint64(),
	}
}

// from256 converts x, which must be in [0, 2^256), to a Uint256.
func from256(x *big.Int) Uint256 {
	if x.Sign() < 0 || x.Cmp(bigTwo256) >= 0 {
		panic(fmt.Sprintf("%v does not fit 256 bits", x))
	}
	m := new(big.Int).Sub(bigTwo128, bigOne)
	return Uint256{
		Hi: from128(new(big.Int).And(new(big.Int).Rsh(x, 128), m)),
		Lo: from128(new(big.Int).And(x, m)),
	}
}

func eq128(t testing.TB, what string, got Uint128, want *big.Int) {
	t.Helper()
	if bigOf(got).Cmp(want) != 0 {
		t.Errorf("%s = %#x:%#x, want %v", what, got.Hi, got.Lo, want)
	}
}

func eq256(t testing.TB, what string, got Uint256, want *big.Int) {
	t.Helper()
	if bigOf256(got).Cmp(want) != 0 {
		t.Errorf("%s = %#x:%#x:%#x:%#x, want %v", what, got.Hi.Hi, got.Hi.Lo, got.Lo.Hi, got.Lo.Lo, want)
	}
}

func eqCarry(t testing.TB, what string, got uint64, want bool) {
	t.Helper()
	if (got != 0) != want || got > 1 {
		t.Errorf("%s = %d, want %t", what, got, want)
	}
}

// edge128 holds the 128-bit values at which carries, borrows, shifts,
// digit counts and quotient estimates change behaviour.
func edge128() []Uint128 {
	var vs []Uint128
	for _, hi := range []uint64{0, 1, 2, 1 << 62, 1 << 63, 1<<63 + 1, math.MaxUint64 - 1, math.MaxUint64} {
		for _, lo := range []uint64{0, 1, 2, 9, 10, 1 << 32, 1<<63 - 1, 1 << 63, math.MaxUint64 - 1, math.MaxUint64} {
			vs = append(vs, Uint128{Hi: hi, Lo: lo})
		}
	}
	for _, n := range []int{18, 19, 20, 33, 34, 37, 38} { // around the powers of ten that bound coefficients
		p := bigPow10(n)
		vs = append(vs, from128(p), from128(new(big.Int).Sub(p, bigOne)))
	}
	return vs
}

func TestU128AddSub(t *testing.T) {
	vs := edge128()
	for _, a := range vs {
		for _, b := range vs {
			checkU128AddSub(t, a, b)
		}
	}
}

func checkU128AddSub(t *testing.T, a, b Uint128) {
	t.Helper()
	A, B := bigOf(a), bigOf(b)

	sum, carry := u128Add(a, b)
	S := new(big.Int).Add(A, B)
	eq128(t, "u128Add sum", sum, new(big.Int).Mod(S, bigTwo128))
	eqCarry(t, "u128Add carry", carry, S.Cmp(bigTwo128) >= 0)

	diff, borrow := u128Sub(a, b)
	D := new(big.Int).Sub(A, B)
	eq128(t, "u128Sub diff", diff, new(big.Int).Mod(D, bigTwo128))
	eqCarry(t, "u128Sub borrow", borrow, D.Sign() < 0)

	want := A.Cmp(B)
	if got := u128Cmp(a, b); got != want {
		t.Errorf("u128Cmp(%v, %v) = %d, want %d", A, B, got, want)
	}
	if got := u128Lt(a, b); got != (want < 0) {
		t.Errorf("u128Lt(%v, %v) = %t", A, B, got)
	}
}

func FuzzU128AddSub(f *testing.F) {
	for _, a := range edge128() {
		f.Add(a.Hi, a.Lo, ^a.Hi, ^a.Lo)
		f.Add(a.Hi, a.Lo, a.Hi, a.Lo)
		f.Add(a.Hi, a.Lo, uint64(0), uint64(1))
	}
	f.Fuzz(func(t *testing.T, ah, al, bh, bl uint64) {
		checkU128AddSub(t, Uint128{ah, al}, Uint128{bh, bl})
	})
}

func checkU128Shift(t *testing.T, a Uint128, n uint) {
	t.Helper()
	A := bigOf(a)
	l := new(big.Int).Lsh(A, n)
	eq128(t, "u128Shl", u128Shl(a, n), l.Mod(l, bigTwo128))
	eq128(t, "u128Shr", u128Shr(a, n), new(big.Int).Rsh(A, n))
	if got, want := u128Len(a), A.BitLen(); got != want {
		t.Errorf("u128Len(%v) = %d, want %d", A, got, want)
	}
}

func TestU128Shift(t *testing.T) {
	for _, a := range edge128() {
		for _, n := range []uint{0, 1, 2, 31, 32, 63, 64, 65, 127, 128, 129, 191, 255, 256, 1000} {
			checkU128Shift(t, a, n)
		}
	}
}

func FuzzU128Shift(f *testing.F) {
	for _, a := range edge128() {
		for _, n := range []uint16{0, 1, 63, 64, 65, 127, 128, 129, 300} {
			f.Add(a.Hi, a.Lo, n)
		}
	}
	f.Fuzz(func(t *testing.T, hi, lo uint64, n uint16) {
		checkU128Shift(t, Uint128{hi, lo}, uint(n))
	})
}

func checkU128Mul(t *testing.T, a, b Uint128) {
	t.Helper()
	A, B := bigOf(a), bigOf(b)

	// The full 128×64 product, and in particular its high word: the
	// wrapping product is what the strawman returned (audit R1).
	p, hi := u128Mul64(a, b.Lo)
	P := new(big.Int).Mul(A, new(big.Int).SetUint64(b.Lo))
	got := new(big.Int).SetUint64(hi)
	got.Lsh(got, 128)
	got.Or(got, bigOf(p))
	if got.Cmp(P) != 0 {
		t.Errorf("u128Mul64(%v, %d) = %d:%v, want %v", A, b.Lo, hi, bigOf(p), P)
	}

	eq256(t, "u128Mul", u128Mul(a, b), new(big.Int).Mul(A, B))
}

func TestU128Mul(t *testing.T) {
	vs := edge128()
	for _, a := range vs {
		for _, b := range vs {
			checkU128Mul(t, a, b)
		}
	}
}

func FuzzU128Mul(f *testing.F) {
	for _, a := range edge128() {
		f.Add(a.Hi, a.Lo, ^a.Hi, ^a.Lo)
		f.Add(a.Hi, a.Lo, a.Hi, a.Lo)
		f.Add(a.Hi, a.Lo, uint64(0), uint64(math.MaxUint64))
	}
	f.Fuzz(func(t *testing.T, ah, al, bh, bl uint64) {
		checkU128Mul(t, Uint128{ah, al}, Uint128{bh, bl})
	})
}

func checkU128Div(t *testing.T, a, d Uint128) {
	t.Helper()
	A, D := bigOf(a), bigOf(d)
	if d == (Uint128{}) {
		mustPanic(t, "u128Div by zero", func() { u128Div(a, d) })
		return
	}
	q, r := new(big.Int).DivMod(A, D, new(big.Int))
	gq, gr := u128Div(a, d)
	eq128(t, "u128Div q", gq, q)
	eq128(t, "u128Div r", gr, r)

	if d.Hi == 0 {
		q64, r64 := u128Div64(a, d.Lo)
		eq128(t, "u128Div64 q", q64, q)
		if r64 != r.Uint64() {
			t.Errorf("u128Div64(%v, %v) remainder = %d, want %v", A, D, r64, r)
		}
	}
}

func TestU128Div(t *testing.T) {
	vs := edge128()
	for _, a := range vs {
		for _, d := range vs {
			checkU128Div(t, a, d)
		}
	}
	mustPanic(t, "u128Div64 by zero", func() { u128Div64(Uint128{Lo: 1}, 0) })
}

func FuzzU128Div(f *testing.F) {
	for _, a := range edge128() {
		f.Add(a.Hi, a.Lo, uint64(0), uint64(1))
		f.Add(a.Hi, a.Lo, uint64(0), uint64(10))
		f.Add(a.Hi, a.Lo, uint64(0), uint64(math.MaxUint64))
		f.Add(a.Hi, a.Lo, uint64(1), uint64(0))
		f.Add(a.Hi, a.Lo, a.Hi, a.Lo)
		f.Add(a.Hi, a.Lo, uint64(0), uint64(0))
	}
	f.Fuzz(func(t *testing.T, ah, al, dh, dl uint64) {
		checkU128Div(t, Uint128{ah, al}, Uint128{dh, dl})
	})
}

// edge256 holds 256-bit values around the word and half boundaries.
func edge256() []Uint256 {
	var vs []Uint256
	for _, h := range edge128() {
		for _, l := range []Uint128{{}, {Lo: 1}, {Hi: 1 << 63}, {Hi: math.MaxUint64, Lo: math.MaxUint64}} {
			vs = append(vs, Uint256{Hi: h, Lo: l})
		}
	}
	return vs
}

func checkU256Ops(t *testing.T, a, b Uint256, n uint) {
	t.Helper()
	A, B := bigOf256(a), bigOf256(b)

	sum, carry := u256Add(a, b)
	S := new(big.Int).Add(A, B)
	eq256(t, "u256Add sum", sum, new(big.Int).Mod(S, bigTwo256))
	eqCarry(t, "u256Add carry", carry, S.Cmp(bigTwo256) >= 0)

	diff, borrow := u256Sub(a, b)
	D := new(big.Int).Sub(A, B)
	eq256(t, "u256Sub diff", diff, new(big.Int).Mod(D, bigTwo256))
	eqCarry(t, "u256Sub borrow", borrow, D.Sign() < 0)

	want := A.Cmp(B)
	if got := u256Cmp(a, b); got != want {
		t.Errorf("u256Cmp(%v, %v) = %d, want %d", A, B, got, want)
	}
	if got := u256Lt(a, b); got != (want < 0) {
		t.Errorf("u256Lt(%v, %v) = %t", A, B, got)
	}

	l := new(big.Int).Lsh(A, n)
	eq256(t, "u256Shl", u256Shl(a, n), l.Mod(l, bigTwo256))
	eq256(t, "u256Shr", u256Shr(a, n), new(big.Int).Rsh(A, n))
	if got, want := u256Len(a), A.BitLen(); got != want {
		t.Errorf("u256Len(%v) = %d, want %d", A, got, want)
	}
}

func TestU256Ops(t *testing.T) {
	vs := edge256()
	rng := rand.New(rand.NewPCG(1, 2))
	for _, a := range vs {
		b := vs[rng.IntN(len(vs))]
		for _, n := range []uint{0, 1, 63, 64, 65, 127, 128, 129, 191, 192, 255, 256, 257, 1000} {
			checkU256Ops(t, a, b, n)
		}
		checkU256Ops(t, a, a, 0)
	}
}

func FuzzU256Ops(f *testing.F) {
	for i, a := range edge256() {
		b := edge256()[(i*7+3)%len(edge256())]
		f.Add(a.Hi.Hi, a.Hi.Lo, a.Lo.Hi, a.Lo.Lo, b.Hi.Hi, b.Hi.Lo, b.Lo.Hi, b.Lo.Lo, uint16(i*37%300))
		f.Add(a.Hi.Hi, a.Hi.Lo, a.Lo.Hi, a.Lo.Lo, a.Hi.Hi, a.Hi.Lo, a.Lo.Hi, a.Lo.Lo, uint16(i%3*64))
	}
	f.Fuzz(func(t *testing.T, a3, a2, a1, a0, b3, b2, b1, b0 uint64, n uint16) {
		a := Uint256{Uint128{a3, a2}, Uint128{a1, a0}}
		b := Uint256{Uint128{b3, b2}, Uint128{b1, b0}}
		checkU256Ops(t, a, b, uint(n))
	})
}

// qEstimates simulates, with math/big, the two quotient-word estimates of
// u256Div128 for a divisor d with a non-zero high word and n.Hi < d, and
// returns by how much each exceeds the true quotient word. A non-zero entry
// is a case in which the add-back correction of div3by2 runs.
func qEstimates(n Uint256, d Uint128) (excess [2]int) {
	s := uint(bits.LeadingZeros64(d.Hi))
	N := new(big.Int).Lsh(bigOf256(n), s)
	V := new(big.Int).Lsh(bigOf(d), s)
	v1 := new(big.Int).Rsh(V, 64)
	maxWord := new(big.Int).SetUint64(math.MaxUint64)
	w0 := new(big.Int).And(N, maxWord)
	u := new(big.Int).Rsh(N, 64) // (w3, w2, w1)
	for step := range excess {
		qhat := new(big.Int).Div(new(big.Int).Rsh(u, 64), v1)
		if qhat.Cmp(maxWord) > 0 {
			qhat = maxWord
		}
		q, r := new(big.Int).DivMod(u, V, new(big.Int))
		excess[step] = int(new(big.Int).Sub(qhat, q).Int64())
		u = new(big.Int).Lsh(r, 64)
		u.Or(u, w0)
	}
	return excess
}

// addBackSeeds returns dividend and divisor pairs for which the estimate of
// a quotient word is too large, so that div3by2 adds back: want1 whose
// worst excess is 1 and want2 whose worst excess is 2. The search is a
// fixed pseudo-random one, so the seeds are the same on every run;
// TestDivSeedsTriggerAddBack re-derives their excess with qEstimates, which
// shares no code with the division, so they stay seeds for the add-back path
// if the division changes.
func addBackSeeds(want1, want2 int) (ns []Uint256, ds []Uint128) {
	rng := rand.New(rand.NewPCG(20261030, 128))
	var got1, got2 int
	for range 1 << 22 {
		if got1 >= want1 && got2 >= want2 {
			break
		}
		d := Uint128{Hi: rng.Uint64() | 1<<63, Lo: rng.Uint64()}
		if rng.IntN(2) == 0 {
			// A divisor barely over 2^127 makes the estimate far too large
			// far more often.
			d.Hi = 1<<63 | rng.Uint64()>>uint(1+rng.IntN(62))
		}
		n := Uint256{
			Hi: Uint128{Hi: rng.Uint64() % (d.Hi + 1), Lo: rng.Uint64()},
			Lo: Uint128{Hi: rng.Uint64(), Lo: rng.Uint64()},
		}
		if !u128Lt(n.Hi, d) {
			continue
		}
		e := qEstimates(n, d)
		worst := max(e[0], e[1])
		switch {
		case worst == 1 && got1 < want1:
			got1++
		case worst >= 2 && got2 < want2:
			got2++
		default:
			continue
		}
		ns = append(ns, n)
		ds = append(ds, d)
	}
	return ns, ds
}

// divSeeds returns (n, d) pairs for the division tests and fuzz targets.
// They are chosen for the places a long division goes wrong: divisors with
// a zero high word, normalised and unnormalised divisors, dividends just
// under, at and just over a multiple of the divisor, quotients at the
// limit of 128 bits and one past it, and the add-back cases of
// addBackSeeds.
func divSeeds() (ns []Uint256, ds []Uint128) {
	add := func(n Uint256, d Uint128) {
		ns = append(ns, n)
		ds = append(ds, d)
	}
	max128 := Uint128{math.MaxUint64, math.MaxUint64}
	divisors := []Uint128{
		{Lo: 1}, {Lo: 10}, {Lo: 1<<63 + 1}, {Lo: math.MaxUint64}, // zero high word
		{Hi: 1}, {Hi: 1, Lo: 1}, {Hi: 1, Lo: math.MaxUint64}, {Hi: 2, Lo: 3},
		{Hi: 1 << 62, Lo: 5},
		{Hi: 1 << 63},        // normalised, zero low word
		{Hi: 1 << 63, Lo: 1}, // smallest divisor with the top bit set, and a low word
		{Hi: 1<<63 | 1<<62, Lo: math.MaxUint64},
		{Hi: math.MaxUint64},
		{Hi: math.MaxUint64, Lo: math.MaxUint64},
		{Hi: math.MaxUint64, Lo: math.MaxUint64 - 1},
		{Hi: 0x7FFFFFFFFFFFFFFF, Lo: math.MaxUint64}, // not normalised, all ones below the top bit
		from128(bigPow10(19)), from128(bigPow10(34)), from128(bigPow10(38)),
		{}, // zero: nothing divides by it, and nothing may panic in u256Div128
	}
	multipliers := []*big.Int{
		big.NewInt(1), big.NewInt(2), big.NewInt(3),
		new(big.Int).SetUint64(math.MaxUint64),
		new(big.Int).Add(bigTwo64, bigOne),
		new(big.Int).Lsh(bigOne, 127),
		new(big.Int).Sub(bigTwo128, big.NewInt(2)),
		new(big.Int).Sub(bigTwo128, bigOne), // the largest quotient
		bigTwo128,                           // one past it
	}
	for _, d := range divisors {
		D := bigOf(d)
		for _, m := range multipliers {
			// n = m×d + e for e = -1, 0, 1 and d-1: just under a multiple,
			// on it, just over it, and as far over as a remainder goes.
			base := new(big.Int).Mul(m, D)
			for _, e := range []*big.Int{big.NewInt(-1), big.NewInt(0), big.NewInt(1), new(big.Int).Sub(D, bigOne)} {
				n := new(big.Int).Add(base, e)
				if n.Sign() < 0 || n.Cmp(bigTwo256) >= 0 {
					continue
				}
				add(from256(n), d)
			}
		}
		hi, _ := u128Sub(d, Uint128{Lo: 1})
		add(Uint256{Hi: hi, Lo: max128}, d) // the largest n with a 128-bit quotient
		add(Uint256{Hi: d}, d)              // the smallest n without one
		add(Uint256{Hi: max128, Lo: max128}, d)
	}
	add(Uint256{Lo: Uint128{Lo: 5}}, Uint128{})
	an, ad := addBackSeeds(12, 6)
	for i := range an {
		add(an[i], ad[i])
	}
	return ns, ds
}

func checkU256Div(t *testing.T, n Uint256, d Uint128) {
	t.Helper()
	N, D := bigOf256(n), bigOf(d)
	fits := d != (Uint128{}) && bigOf(n.Hi).Cmp(D) < 0

	q, r, ok := u256Div128(n, d)
	if ok != fits {
		t.Errorf("u256Div128(%v, %v) ok = %t, want %t", N, D, ok, fits)
	}
	if !ok {
		// Nothing fits, so nothing is returned, in particular no truncated
		// quotient (audit R2).
		if q != (Uint128{}) || r != (Uint128{}) {
			t.Errorf("u256Div128(%v, %v) = %v, %v with ok false; want zeros", N, D, q, r)
		}
	} else {
		wq, wr := new(big.Int).DivMod(N, D, new(big.Int))
		eq128(t, "u256Div128 q", q, wq)
		eq128(t, "u256Div128 r", r, wr)
	}

	if d == (Uint128{}) {
		mustPanic(t, "u256DivMod128 by zero", func() { u256DivMod128(n, d) })
		return
	}
	wq, wr := new(big.Int).DivMod(N, D, new(big.Int))
	fq, fr := u256DivMod128(n, d)
	eq256(t, "u256DivMod128 q", fq, wq)
	eq128(t, "u256DivMod128 r", fr, wr)
}

func TestU256Div(t *testing.T) {
	ns, ds := divSeeds()
	for i := range ns {
		checkU256Div(t, ns[i], ds[i])
	}
}

func FuzzU256Div128(f *testing.F) {
	ns, ds := divSeeds()
	for i := range ns {
		n, d := ns[i], ds[i]
		f.Add(n.Hi.Hi, n.Hi.Lo, n.Lo.Hi, n.Lo.Lo, d.Hi, d.Lo)
	}
	f.Fuzz(func(t *testing.T, n3, n2, n1, n0, d1, d0 uint64) {
		n := Uint256{Uint128{n3, n2}, Uint128{n1, n0}}
		d := Uint128{d1, d0}
		checkU256Div(t, n, d)
		if d != (Uint128{}) {
			// Random input rarely fits a 128-bit quotient when d is small, so
			// also divide n reduced to fit: its high half modulo d.
			_, rh := u128Div(n.Hi, d)
			checkU256Div(t, Uint256{Hi: rh, Lo: n.Lo}, d)
		}
	})
}

// TestDivSeedsTriggerAddBack checks that the add-back seeds do what they
// are for, using the simulation of qEstimates and not the division under
// test: at least 12 estimates one too large and 6 two too large.
func TestDivSeedsTriggerAddBack(t *testing.T) {
	ns, ds := addBackSeeds(12, 6)
	var ones, twos int
	for i := range ns {
		e := qEstimates(ns[i], ds[i])
		switch worst := max(e[0], e[1]); {
		case worst == 1:
			ones++
		case worst >= 2:
			twos++
		}
	}
	if ones < 12 || twos < 6 {
		t.Errorf("add-back seeds: %d with excess 1 (want 12), %d with excess 2 (want 6)", ones, twos)
	}
}

func mustPanic(t testing.TB, what string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		if recover() == nil {
			t.Errorf("%s did not panic", what)
		}
	}()
	f()
}

// The primitives allocate nothing. The results are kept in package
// variables so that the compiler cannot discard the calls.
var (
	sinkU128 Uint128
	sinkU256 Uint256
	sinkU64  uint64
	sinkInt  int
	sinkBool bool
)

func TestWideZeroAllocs(t *testing.T) {
	a := Uint128{Hi: 0x0123456789ABCDEF, Lo: 0xFEDCBA9876543210}
	b := Uint128{Hi: 0x8000000000000001, Lo: 0x1122334455667788}
	n := Uint256{Hi: Uint128{Hi: 0x7000000000000000, Lo: 1}, Lo: a}
	w := Uint256{Hi: a, Lo: b}
	tests := []struct {
		name string
		f    func()
	}{
		{"u128Add", func() { sinkU128, sinkU64 = u128Add(a, b) }},
		{"u128Sub", func() { sinkU128, sinkU64 = u128Sub(a, b) }},
		{"u128Cmp", func() { sinkInt = u128Cmp(a, b) }},
		{"u128Shl", func() { sinkU128 = u128Shl(a, 77) }},
		{"u128Shr", func() { sinkU128 = u128Shr(a, 77) }},
		{"u128Mul64", func() { sinkU128, sinkU64 = u128Mul64(a, b.Lo) }},
		{"u128Mul", func() { sinkU256 = u128Mul(a, b) }},
		{"u128Div64", func() { sinkU128, sinkU64 = u128Div64(a, b.Lo) }},
		{"u128Div", func() { sinkU128, sinkU128 = u128Div(a, b) }},
		{"u256Add", func() { sinkU256, sinkU64 = u256Add(w, n) }},
		{"u256Sub", func() { sinkU256, sinkU64 = u256Sub(w, n) }},
		{"u256Cmp", func() { sinkInt = u256Cmp(w, n) }},
		{"u256Shl", func() { sinkU256 = u256Shl(w, 77) }},
		{"u256Shr", func() { sinkU256 = u256Shr(w, 177) }},
		{"u256Div128", func() { sinkU128, sinkU128, sinkBool = u256Div128(n, b) }},
		{"u256DivMod128", func() { sinkU256, sinkU128 = u256DivMod128(w, b) }},
	}
	for _, tt := range tests {
		if got := testing.AllocsPerRun(100, tt.f); got != 0 {
			t.Errorf("%s allocates %v times per call, want 0", tt.name, got)
		}
	}
}
