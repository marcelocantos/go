// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package constdecimal_test

import (
	"go/constant"
	"go/token"
	"internal/bid"
	"internal/constdecimal"
	"testing"
)

func lit(s string) constant.Value {
	return constant.MakeFromLiteral(s, token.FLOAT, 0)
}

func neg(v constant.Value) constant.Value {
	return constant.UnaryOp(token.SUB, v, 0)
}

// The values a source program can't produce, because the type checker rejects
// them, must still reach the kernel's default result: an infinity of the
// value's sign. The encodings are written by hand.
func TestEncodeOutOfRange(t *testing.T) {
	const (
		inf  = 0x7800000000000000
		sign = 1 << 63
	)
	tests := []struct {
		v       constant.Value
		want64  uint64 // decimal64 bits
		over128 bool   // decimal128 is an infinity too
	}{
		{v: lit("1e385"), want64: inf},
		{v: neg(lit("1e385")), want64: sign | inf},
		{v: lit("1e6145"), want64: inf, over128: true},
		{v: lit("1e100000"), want64: inf, over128: true},
		{v: neg(lit("1e100000")), want64: sign | inf, over128: true},
	}
	for _, test := range tests {
		bits, ok := constdecimal.Encode(test.v, constdecimal.Decimal64)
		if ok || bits.Lo != test.want64 {
			t.Errorf("decimal64 %v = %#x, %v, want %#x, false", test.v, bits.Lo, ok, test.want64)
		}
		bits, ok = constdecimal.Encode(test.v, constdecimal.Decimal128)
		want := uint64(0)
		if constant.Sign(test.v) < 0 {
			want = sign
		}
		switch {
		case test.over128:
			if ok || bits.Lo != 0 || bits.Hi != want|inf {
				t.Errorf("decimal128 %v = %#x, %#x, want infinity", test.v, bits.Lo, bits.Hi)
			}
		case !ok || bits.Hi&inf == inf || bits.Hi&sign != want:
			t.Errorf("decimal128 %v = %#x, %#x, want a finite value of that sign", test.v, bits.Lo, bits.Hi)
		}
	}
}

// Each case gives the coefficient and exponent the encoding must have,
// worked out by hand from the rules in the package documentation.
func TestEncode(t *testing.T) {
	quo := func(x, y constant.Value) constant.Value { return constant.BinaryOp(x, token.QUO, y) }
	mul := func(x, y constant.Value) constant.Value { return constant.BinaryOp(x, token.MUL, y) }
	tests := []struct {
		name  string
		v     constant.Value
		coeff uint64
		exp   int
	}{
		{"literal keeps its quantum", lit("1.50"), 150, -2},
		{"exponent literal", lit("1.5e3"), 15, 2},
		{"zero keeps its quantum", lit("0.00"), 0, -2},
		{"integer has quantum 0", constant.MakeInt64(100), 100, 0},
		{"exact product keeps the sum of quanta", mul(lit("1.50"), lit("2")), 300, -2},
		{"exact quotient", quo(lit("1.00"), lit("4")), 25, -2},
		{"inexact quotient keeps every digit", quo(lit("1.0"), lit("3.0")), 3333333333333333, -16},
		{"quantum too fine uses the closest exponent", lit("1.000000000000000000"), 1000000000000000, -15},
		{"no quantum and integral uses exponent 0", constant.MakeFloat64(3), 3, 0},
		{"no quantum uses the smallest coefficient", constant.MakeFloat64(0.5), 5, -1},
		{"rounds ties to even", lit("1.0000000000000005"), 1000000000000000, -15},
		{"huge literal is exact", lit("1E+300"), 1, 300},
	}
	for _, test := range tests {
		bits, ok := constdecimal.Encode(test.v, constdecimal.Decimal64)
		_, n, coeff, exp := bid.Unpack64(bits.Lo)
		if !ok || n || coeff != test.coeff || exp != test.exp {
			t.Errorf("%s: Encode(%v) = %d×10^%d (neg %v, ok %v), want %d×10^%d", test.name, test.v, coeff, exp, n, ok, test.coeff, test.exp)
		}
	}
}

func TestRoundAndDecode(t *testing.T) {
	v, ok := constdecimal.Round(lit("1.0000000000000005"), constdecimal.Decimal64)
	if !ok {
		t.Fatal("Round failed")
	}
	if e, ok := constdecimal.Quantum(v); !ok || e != -15 || constant.Compare(v, token.NEQ, constant.MakeInt64(1)) {
		t.Errorf("Round(1.0000000000000005) = %v with quantum %d, %v; want 1 with quantum -15", v, e, ok)
	}
	v, _ = constdecimal.Round(neg(lit("0.00")), constdecimal.Decimal64)
	if e, ok := constdecimal.Quantum(v); !ok || e != -2 || constant.Sign(v) != 0 {
		t.Errorf("Round(-0.00) = %v with quantum %d, %v; want 0 with quantum -2", v, e, ok)
	}
}

// Typed decimal arithmetic rounds each operation as at run time and takes the
// preferred exponent. The expected results are worked out by hand.
func TestBinaryOp(t *testing.T) {
	d64 := func(s string) constant.Value {
		v, ok := constdecimal.Round(lit(s), constdecimal.Decimal64)
		if !ok {
			t.Fatalf("Round(%s) failed", s)
		}
		return v
	}
	tests := []struct {
		x     string
		op    token.Token
		y     string
		coeff uint64
		exp   int
	}{
		{"1.50", token.MUL, "2", 300, -2},
		{"1.50", token.ADD, "0.1", 160, -2},
		{"1", token.QUO, "3", 3333333333333333, -16},
		{"0.3333333333333333", token.MUL, "3", 9999999999999999, -16},
		{"1.00", token.QUO, "4", 25, -2},
		{"5.00", token.SUB, "5", 0, -2},
	}
	for _, test := range tests {
		v, ok := constdecimal.BinaryOp(d64(test.x), test.op, d64(test.y), constdecimal.Decimal64)
		if !ok {
			t.Errorf("%s %v %s: not finite", test.x, test.op, test.y)
			continue
		}
		bits, _ := constdecimal.Encode(v, constdecimal.Decimal64)
		_, _, coeff, exp := bid.Unpack64(bits.Lo)
		if coeff != test.coeff || exp != test.exp {
			t.Errorf("%s %v %s = %d×10^%d, want %d×10^%d", test.x, test.op, test.y, coeff, exp, test.coeff, test.exp)
		}
	}
	if _, ok := constdecimal.BinaryOp(d64("9e384"), token.MUL, d64("10"), constdecimal.Decimal64); ok {
		t.Errorf("9e384 * 10 is finite, want overflow")
	}
}
