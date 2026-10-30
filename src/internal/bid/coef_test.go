// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import (
	"bytes"
	"internal/testenv"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

// Tests of the coefficient vocabulary (coef64.go, coef128.go), the powers
// of ten, the generated function, and the generator itself.

// TestGenerated regenerates the generated files into a scratch directory and
// fails on any difference from the committed ones: a stale or hand-edited
// z*.go file, or a change to the template or generator that was not
// followed by go generate.
func TestGenerated(t *testing.T) {
	testenv.MustHaveGoRun(t)
	dir := t.TempDir()
	cmd := testenv.Command(t, testenv.GoToolPath(t), "run", "gen_arith.go", "-out", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go run gen_arith.go: %v\n%s", err, out)
	}
	// Compare in both directions, so that a generated file whose template
	// was removed is reported too.
	generated, err := filepath.Glob(filepath.Join(dir, "z*.go"))
	if err != nil {
		t.Fatal(err)
	}
	committed, err := filepath.Glob("z*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != len(committed) {
		t.Errorf("the generator writes %d files and %d are committed: run go generate in internal/bid", len(generated), len(committed))
	}
	for _, path := range generated {
		name := filepath.Base(path)
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(name)
		if err != nil {
			t.Errorf("%s is missing: run go generate in internal/bid", name)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is out of date: run go generate in internal/bid", name)
		}
	}
}

// TestPow10Tables checks the three tables against math/big: every entry,
// and that each table stops at the last power of ten below its width.
func TestPow10Tables(t *testing.T) {
	check := func(name string, n int, got func(k int) *big.Int, limit *big.Int) {
		for k := 0; k < n; k++ {
			if want := bigPow10(k); got(k).Cmp(want) != 0 {
				t.Errorf("%s[%d] = %v, want %v", name, k, got(k), want)
			}
		}
		if last := bigPow10(n - 1); last.Cmp(limit) >= 0 || new(big.Int).Mul(last, big.NewInt(10)).Cmp(limit) < 0 {
			t.Errorf("%s has %d entries; 10^%d is not the last power of ten below %v", name, n, n-1, limit)
		}
	}
	check("pow10tab64", len(pow10tab64), func(k int) *big.Int { return new(big.Int).SetUint64(pow10tab64[k]) }, bigTwo64)
	check("pow10tab128", len(pow10tab128), func(k int) *big.Int { return bigOf(pow10tab128[k]) }, bigTwo128)
	check("pow10tab256", len(pow10tab256), func(k int) *big.Int { return bigOf256(pow10tab256[k]) }, bigTwo256)
}

func bigDigits(x *big.Int) int {
	if x.Sign() == 0 {
		return 0
	}
	return len(x.String())
}

func big64(c uint64) *big.Int { return new(big.Int).SetUint64(c) }

// edge64 holds the 64-bit values at which digit counts and carries change.
func edge64() []uint64 {
	vs := []uint64{0, 1, 2, 5, 9, 10, 11, 99, 100, math.MaxUint64, math.MaxUint64 - 1, 1 << 63, 1<<63 - 1, 1 << 32, MaxCoeff64, MaxCoeff64 + 1, MaxCoeff64 - 1}
	for k := 0; k < len(pow10tab64); k++ {
		vs = append(vs, pow10tab64[k], pow10tab64[k]-1, pow10tab64[k]+1)
	}
	for b := 0; b < 64; b++ {
		vs = append(vs, 1<<b, 1<<b-1)
	}
	return vs
}

func checkCoef64(t *testing.T, a, b uint64, k int) {
	t.Helper()
	A, B := big64(a), big64(b)

	if got, want := cIsZero64(a), a == 0; got != want {
		t.Errorf("cIsZero64(%d) = %t", a, got)
	}
	if got, want := cCmp64(a, b), A.Cmp(B); got != want {
		t.Errorf("cCmp64(%d, %d) = %d, want %d", a, b, got, want)
	}
	sum, carry := cAdd64(a, b)
	S := new(big.Int).Add(A, B)
	if big64(sum).Cmp(new(big.Int).Mod(S, bigTwo64)) != 0 || carry != (S.Cmp(bigTwo64) >= 0) {
		t.Errorf("cAdd64(%d, %d) = %d, %t", a, b, sum, carry)
	}
	diff, borrow := cSub64(a, b)
	D := new(big.Int).Sub(A, B)
	if big64(diff).Cmp(new(big.Int).Mod(D, bigTwo64)) != 0 || borrow != (D.Sign() < 0) {
		t.Errorf("cSub64(%d, %d) = %d, %t", a, b, diff, borrow)
	}
	if got, want := cOdd64(a), a&1 == 1; got != want {
		t.Errorf("cOdd64(%d) = %t", a, got)
	}
	if got, want := cDigits64(a), bigDigits(A); got != want {
		t.Errorf("cDigits64(%d) = %d, want %d", a, got, want)
	}
	if got, want := cGtMax64(a), A.Cmp(big64(MaxCoeff64)) > 0; got != want {
		t.Errorf("cGtMax64(%d) = %t", a, got)
	}
	if got, want := cMulFull64(a, b), new(big.Int).Mul(A, B); bigOf(got).Cmp(want) != 0 {
		t.Errorf("cMulFull64(%d, %d) = %v, want %v", a, b, bigOf(got), want)
	}
	if got, want := cLenFull64(cMulFull64(a, b)), bigDigits(new(big.Int).Mul(A, B)); got != want {
		t.Errorf("cLenFull64(%d × %d) = %d, want %d", a, b, got, want)
	}

	if b != 0 {
		p := cMulFull64(a, a)
		if got, want := cModFull64(p, b), new(big.Int).Mod(bigOf(p), B); big64(got).Cmp(want) != 0 {
			t.Errorf("cModFull64(%d × %d, %d) = %d, want %v", a, a, b, got, want)
		}
	}

	k %= len(pow10tab64) // 0 <= k <= 19
	P := bigPow10(k)
	if got := cPow1064(k); big64(got).Cmp(P) != 0 {
		t.Errorf("cPow1064(%d) = %d", k, got)
	}
	q, r := cDivPow1064(a, k)
	wq, wr := new(big.Int).DivMod(A, P, new(big.Int))
	if big64(q).Cmp(wq) != 0 || big64(r).Cmp(wr) != 0 {
		t.Errorf("cDivPow1064(%d, %d) = %d, %d, want %v, %v", a, k, q, r, wq, wr)
	}
	if k >= 1 {
		if got, want := cHalf64(k), new(big.Int).Mul(big.NewInt(5), bigPow10(k-1)); big64(got).Cmp(want) != 0 {
			t.Errorf("cHalf64(%d) = %d, want %v", k, got, want)
		}
	}
	if M := new(big.Int).Mul(A, P); M.Cmp(bigTwo64) < 0 {
		if got := cMulPow1064(a, k); big64(got).Cmp(M) != 0 {
			t.Errorf("cMulPow1064(%d, %d) = %d, want %v", a, k, got, M)
		}
	} else {
		mustPanic(t, "cMulPow1064 overflow", func() { cMulPow1064(a, k) })
	}

	// cReduce64 of the product, which fits when the quotient does.
	p := cMulFull64(a, b)
	Q, R := new(big.Int).DivMod(new(big.Int).Mul(A, B), P, new(big.Int))
	if Q.Cmp(bigTwo64) < 0 {
		if got, sticky := cReduce64(p, k); big64(got).Cmp(Q) != 0 || sticky != (R.Sign() != 0) {
			t.Errorf("cReduce64(%d × %d, %d) = %d, %t, want %v, %t", a, b, k, got, sticky, Q, R.Sign() != 0)
		}
	} else {
		mustPanic(t, "cReduce64 quotient overflow", func() { cReduce64(p, k) })
	}
}

func TestCoef64(t *testing.T) {
	vs := edge64()
	for i, a := range vs {
		for j, b := range vs {
			checkCoef64(t, a, b, i+j)
		}
	}
	for k := 0; k < 25; k++ {
		checkCoef64(t, MaxCoeff64, MaxCoeff64, k)
	}
}

func FuzzCoef64(f *testing.F) {
	vs := edge64()
	for i, a := range vs {
		f.Add(a, vs[(i*5+1)%len(vs)], uint8(i))
		f.Add(a, a, uint8(i%20))
	}
	f.Fuzz(func(t *testing.T, a, b uint64, k uint8) {
		checkCoef64(t, a, b, int(k))
	})
}

// edgeCoef128 holds the 128-bit values at which digit counts change, in
// addition to edge128's.
func edgeCoef128() []Uint128 {
	vs := edge128()
	for k := 0; k < len(pow10tab128); k++ {
		p := pow10tab128[k]
		vs = append(vs, p)
		m, _ := u128Sub(p, Uint128{Lo: 1})
		vs = append(vs, m)
		if k < len(pow10tab128)-1 {
			s, _ := u128Add(p, Uint128{Lo: 1})
			vs = append(vs, s)
		}
	}
	for b := 0; b < 128; b++ {
		p := u128Shl(Uint128{Lo: 1}, uint(b))
		m, _ := u128Sub(p, Uint128{Lo: 1})
		vs = append(vs, p, m)
	}
	vs = append(vs, Uint128{MaxCoeff128Hi, MaxCoeff128Lo}, Uint128{MaxCoeff128Hi, MaxCoeff128Lo + 1}, Uint128{MaxCoeff128Hi, MaxCoeff128Lo - 1})
	return vs
}

func checkCoef128(t *testing.T, a, b Uint128, k int) {
	t.Helper()
	A, B := bigOf(a), bigOf(b)

	if got, want := cIsZero128(a), A.Sign() == 0; got != want {
		t.Errorf("cIsZero128(%v) = %t", A, got)
	}
	if got, want := cCmp128(a, b), A.Cmp(B); got != want {
		t.Errorf("cCmp128(%v, %v) = %d, want %d", A, B, got, want)
	}
	sum, carry := cAdd128(a, b)
	S := new(big.Int).Add(A, B)
	if bigOf(sum).Cmp(new(big.Int).Mod(S, bigTwo128)) != 0 || carry != (S.Cmp(bigTwo128) >= 0) {
		t.Errorf("cAdd128(%v, %v) = %v, %t", A, B, bigOf(sum), carry)
	}
	diff, borrow := cSub128(a, b)
	D := new(big.Int).Sub(A, B)
	if bigOf(diff).Cmp(new(big.Int).Mod(D, bigTwo128)) != 0 || borrow != (D.Sign() < 0) {
		t.Errorf("cSub128(%v, %v) = %v, %t", A, B, bigOf(diff), borrow)
	}
	if got, want := cOdd128(a), A.Bit(0) == 1; got != want {
		t.Errorf("cOdd128(%v) = %t", A, got)
	}
	if got, want := cDigits128(a), bigDigits(A); got != want {
		t.Errorf("cDigits128(%v) = %d, want %d", A, got, want)
	}
	if got, want := cGtMax128(a), A.Cmp(new(big.Int).Sub(bigPow10(34), bigOne)) > 0; got != want {
		t.Errorf("cGtMax128(%v) = %t", A, got)
	}
	prod := new(big.Int).Mul(A, B)
	p := cMulFull128(a, b)
	eq256(t, "cMulFull128", p, prod)
	if got, want := cLenFull128(p), bigDigits(prod); got != want {
		t.Errorf("cLenFull128(%v × %v) = %d, want %d", A, B, got, want)
	}

	k1 := k % len(pow10tab128) // 0 <= k1 <= 38
	P := bigPow10(k1)
	eq128(t, "cPow10128", cPow10128(k1), P)
	q, r := cDivPow10128(a, k1)
	wq, wr := new(big.Int).DivMod(A, P, new(big.Int))
	eq128(t, "cDivPow10128 q", q, wq)
	eq128(t, "cDivPow10128 r", r, wr)
	if k1 >= 1 {
		eq128(t, "cHalf128", cHalf128(k1), new(big.Int).Mul(big.NewInt(5), bigPow10(k1-1)))
	}
	if M := new(big.Int).Mul(A, P); M.Cmp(bigTwo128) < 0 {
		eq128(t, "cMulPow10128", cMulPow10128(a, k1), M)
	} else {
		mustPanic(t, "cMulPow10128 overflow", func() { cMulPow10128(a, k1) })
	}

	if m, _ := u128Add(b, Uint128{Lo: 1}); m != (Uint128{}) {
		eq128(t, "cModFull128", cModFull128(p, m), new(big.Int).Mod(prod, bigOf(m)))
	}

	// cReduce128 over its whole range of k, including the multi-step
	// divisions above 10^38 and the zero quotient above 10^77.
	k2 := k % 90
	Q, R := new(big.Int).DivMod(prod, bigPow10(k2), new(big.Int))
	if Q.Cmp(bigTwo128) < 0 {
		gq, sticky := cReduce128(p, k2)
		eq128(t, "cReduce128 q", gq, Q)
		if sticky != (R.Sign() != 0) {
			t.Errorf("cReduce128(%v × %v, %d) sticky = %t, want %t", A, B, k2, sticky, R.Sign() != 0)
		}
	} else {
		mustPanic(t, "cReduce128 quotient overflow", func() { cReduce128(p, k2) })
	}
}

func TestCoef128(t *testing.T) {
	vs := edgeCoef128()
	for i, a := range vs {
		for j, b := range []Uint128{vs[(i*7+1)%len(vs)], vs[(i*13+5)%len(vs)], vs[len(vs)-1-i]} {
			for _, k := range []int{0, 1, 18, 19, 20, 37, 38, 39, 50, 76, 77, 78, 89, i + j} {
				checkCoef128(t, a, b, k)
			}
		}
	}
	m := Uint128{MaxCoeff128Hi, MaxCoeff128Lo}
	for k := 0; k < 90; k++ {
		checkCoef128(t, m, m, k)
	}
}

func FuzzCoef128(f *testing.F) {
	vs := edgeCoef128()
	for i, a := range vs {
		b := vs[(i*5+1)%len(vs)]
		f.Add(a.Hi, a.Lo, b.Hi, b.Lo, uint8(i))
		f.Add(a.Hi, a.Lo, a.Hi, a.Lo, uint8(i%90))
	}
	f.Fuzz(func(t *testing.T, ah, al, bh, bl uint64, k uint8) {
		checkCoef128(t, Uint128{ah, al}, Uint128{bh, bl}, int(k))
	})
}

// TestCoefZeroAllocs asserts that the vocabulary and the generated function
// allocate nothing, for both widths.
func TestCoefZeroAllocs(t *testing.T) {
	a := Uint128{Hi: 0x0001ED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF}
	b := Uint128{Hi: 0x0000000012345678, Lo: 0xFEDCBA9876543210}
	var a64, b64 uint64 = MaxCoeff64, 9876543210987654
	p := cMulFull128(a, b)
	tests := []struct {
		name string
		f    func()
	}{
		{"cDigits64", func() { sinkInt = cDigits64(a64) }},
		{"cMulPow1064", func() { sinkU64 = cMulPow1064(123, 10) }},
		{"cDivPow1064", func() { sinkU64, sinkU64 = cDivPow1064(a64, 7) }},
		{"cMulFull64", func() { sinkU128 = cMulFull64(a64, b64) }},
		{"cReduce64", func() { sinkU64, sinkBool = cReduce64(cMulFull64(a64, b64), 15) }},
		{"cLenFull64", func() { sinkInt = cLenFull64(cMulFull64(a64, b64)) }},
		{"cModFull64", func() { sinkU64 = cModFull64(cMulFull64(a64, b64), 7) }},
		{"cDigits128", func() { sinkInt = cDigits128(a) }},
		{"cMulPow10128", func() { sinkU128 = cMulPow10128(Uint128{Lo: 123}, 30) }},
		{"cDivPow10128", func() { sinkU128, sinkU128 = cDivPow10128(a, 30) }},
		{"cMulFull128", func() { sinkU256 = cMulFull128(a, b) }},
		{"cReduce128", func() { sinkU128, sinkBool = cReduce128(p, 40) }},
		{"cLenFull128", func() { sinkInt = cLenFull128(p) }},
		{"cModFull128", func() { sinkU128 = cModFull128(p, b) }},
	}
	for _, tt := range tests {
		if got := testing.AllocsPerRun(100, tt.f); got != 0 {
			t.Errorf("%s allocates %v times per call, want 0", tt.name, got)
		}
	}
}
