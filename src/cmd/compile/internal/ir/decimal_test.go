// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ir

import (
	"go/constant"
	"go/token"
	"internal/bid"
	"math/big"
	"testing"
)

// The values a source program can't produce, because the type checker rejects
// them, must still reach bid's default result. test/decimal_const.go checks the
// encodings of the constants a program can write.
func TestDecimalConstOutOfRange(t *testing.T) {
	const (
		inf  = 0x7800000000000000
		sign = 1 << 63
	)
	tests := []struct {
		lit     string
		neg     bool
		want64  uint64 // decimal64 bits
		over128 bool   // decimal128 is an infinity too
	}{
		{lit: "1e385", want64: inf},
		{lit: "1e385", neg: true, want64: sign | inf},
		{lit: "1e6145", want64: inf, over128: true},
		{lit: "1e100000", want64: inf, over128: true},
		{lit: "1e100000", neg: true, want64: sign | inf, over128: true},
	}
	for _, test := range tests {
		val := constant.MakeFromLiteral(test.lit, token.FLOAT, 0)
		if test.neg {
			val = constant.UnaryOp(token.SUB, val, 0)
		}
		neg, coeff, exp, sticky := decimalParts(val, "", bid.Precision64, bid.MinExp64, bid.Emax64)
		if bits, _ := bid.Round64(neg, coeff.Uint64(), exp, sticky, bid.ToNearestEven); bits != test.want64 {
			t.Errorf("decimal64 %s (neg %v) = %#x, want %#x", test.lit, test.neg, bits, test.want64)
		}
		neg, coeff, exp, sticky = decimalParts(val, "", bid.Precision128, bid.MinExp128, bid.Emax128)
		high := new(big.Int).Rsh(coeff, 64)
		low := new(big.Int).Sub(coeff, new(big.Int).Lsh(high, 64))
		bits, _ := bid.Round128(neg, bid.Uint128{Hi: high.Uint64(), Lo: low.Uint64()}, exp, sticky, bid.ToNearestEven)
		lo, hi := bits.Lo, bits.Hi
		var want uint64
		if test.neg {
			want = sign
		}
		switch {
		case test.over128:
			if lo != 0 || hi != want|inf {
				t.Errorf("decimal128 %s (neg %v) = %#x, %#x, want infinity", test.lit, test.neg, lo, hi)
			}
		case hi&inf == inf || hi&sign != want:
			t.Errorf("decimal128 %s (neg %v) = %#x, %#x, want a finite value of that sign", test.lit, test.neg, lo, hi)
		}
	}
}

func TestLiteralExponent(t *testing.T) {
	tests := []struct {
		lit string
		exp int
		ok  bool
	}{
		{"", 0, false},
		{"1.50", -2, true},
		{"1_000.50", -2, true},
		{"100", 0, true},
		{".5", -1, true},
		{"5.", 0, true},
		{"1.5e3", 2, true},
		{"1.5E+3", 2, true},
		{"1.5E-3", -4, true},
		{"0755", 0, true},
		{"'a'", 0, true},
		{"'e'", 0, true},
		{"0x1p-2", 0, true},
		{"0X1.8P1", 0, true},
		{"0xe", 0, true},
		{"0o17", 0, true},
		{"0b101", 0, true},
		{"0.00i", -2, true},
		{"1e99999999999999999999", 1 << 30, true},
		{"1e-99999999999999999999", -1 << 30, true},
	}
	for _, test := range tests {
		exp, ok := literalExponent(test.lit)
		if exp != test.exp || ok != test.ok {
			t.Errorf("literalExponent(%q) = %d, %v, want %d, %v", test.lit, exp, ok, test.exp, test.ok)
		}
	}
}
