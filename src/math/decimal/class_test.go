// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package decimal_test

import (
	"math/decimal"
	"runtime"
	"testing"
)

// TestClass128 checks Class and the predicates of IEEE 754-2019 clause
// 5.7.2 on decimal128: one value of each class, and the values on either
// side of the smallest normal magnitude, 1e-6143.
func TestClass128(t *testing.T) {
	zero := decimal128(0)
	tests := []struct {
		x    decimal128
		want decimal.IEEE754Class
	}{
		{decimal.SignalingNaNWithPayload(zero), decimal.ClassSignalingNaN},
		{-decimal.SignalingNaNWithPayload(decimal128(42)), decimal.ClassSignalingNaN},
		{decimal.NaN[decimal128](), decimal.ClassQuietNaN},
		{-decimal.NaNWithPayload(decimal128(42)), decimal.ClassQuietNaN},
		{decimal.Inf[decimal128](-1), decimal.ClassNegativeInfinity},
		{-decimal.MaxDecimal128, decimal.ClassNegativeNormal},
		{-1e-6143, decimal.ClassNegativeNormal},
		{-9.99999999999999999999999999999999e-6144, decimal.ClassNegativeSubnormal},
		{-decimal.SmallestNonzeroDecimal128, decimal.ClassNegativeSubnormal},
		{decimal.Copysign(zero, -1), decimal.ClassNegativeZero},
		{zero, decimal.ClassPositiveZero},
		{0e-6176, decimal.ClassPositiveZero},
		{0e6111, decimal.ClassPositiveZero},
		{decimal.SmallestNonzeroDecimal128, decimal.ClassPositiveSubnormal},
		{9e-6144, decimal.ClassPositiveSubnormal},
		{9.99999999999999999999999999999999e-6144, decimal.ClassPositiveSubnormal},
		{1e-6143, decimal.ClassPositiveNormal},
		{decimal.Quantize(decimal128(1e-6143), decimal.SmallestNonzeroDecimal128), decimal.ClassPositiveNormal},
		{1, decimal.ClassPositiveNormal},
		{decimal.MaxDecimal128, decimal.ClassPositiveNormal},
		{decimal.Inf[decimal128](1), decimal.ClassPositiveInfinity},
	}
	for _, tt := range tests {
		x := tt.x
		if got := decimal.Class(x); got != tt.want {
			t.Errorf("Class(%v) = %v, want %v", x, got, tt.want)
		}
		c := tt.want
		finite := c != decimal.ClassSignalingNaN && c != decimal.ClassQuietNaN &&
			c != decimal.ClassNegativeInfinity && c != decimal.ClassPositiveInfinity
		zeroClass := c == decimal.ClassNegativeZero || c == decimal.ClassPositiveZero
		sub := c == decimal.ClassNegativeSubnormal || c == decimal.ClassPositiveSubnormal
		normal := c == decimal.ClassNegativeNormal || c == decimal.ClassPositiveNormal
		for _, p := range []struct {
			name      string
			got, want bool
		}{
			{"IsFinite", decimal.IsFinite(x), finite},
			{"IsZero", decimal.IsZero(x), zeroClass},
			{"IsSubnormal", decimal.IsSubnormal(x), sub},
			{"IsNormal", decimal.IsNormal(x), normal},
			{"IsCanonical", decimal.IsCanonical(x), true},
		} {
			if p.got != p.want {
				t.Errorf("%s(%v) = %v, want %v", p.name, x, p.got, p.want)
			}
		}
		if got, want := decimal.IsZero(x), x == 0; got != want {
			t.Errorf("IsZero(%v) = %v, but x == 0 is %v", x, got, want)
		}
	}
}

// TestIsCanonical128 checks that a decimal128 stored as UBD is canonical
// even when it is made from a non-canonical BID encoding, whose value is
// zero (IEEE 754-2019 clause 3.5.2).
func TestIsCanonical128(t *testing.T) {
	if runtime.GOARCH == "ppc64le" {
		t.Skip("decimal128 is not stored as UBD on ppc64le")
	}
	for _, b := range [][2]uint64{
		{0x6000000000000000, 7},                  // second form
		{0x0001ed09bead87c0, 0x378d8e6400000000}, // coefficient 10^34
	} {
		x := decimal.BID128frombits(b[0], b[1])
		if !decimal.IsCanonical(x) {
			t.Errorf("IsCanonical(BID128frombits(%#x, %#x)) = false", b[0], b[1])
		}
		if c := decimal.Class(x); c != decimal.ClassPositiveZero {
			t.Errorf("Class(BID128frombits(%#x, %#x)) = %v, want PositiveZero", b[0], b[1], c)
		}
	}
}

func TestIEEE754ClassString(t *testing.T) {
	for c, want := range map[decimal.IEEE754Class]string{
		decimal.ClassSignalingNaN:      "SignalingNaN",
		decimal.ClassQuietNaN:          "QuietNaN",
		decimal.ClassNegativeInfinity:  "NegativeInfinity",
		decimal.ClassNegativeNormal:    "NegativeNormal",
		decimal.ClassNegativeSubnormal: "NegativeSubnormal",
		decimal.ClassNegativeZero:      "NegativeZero",
		decimal.ClassPositiveZero:      "PositiveZero",
		decimal.ClassPositiveSubnormal: "PositiveSubnormal",
		decimal.ClassPositiveNormal:    "PositiveNormal",
		decimal.ClassPositiveInfinity:  "PositiveInfinity",
		10:                             "IEEE754Class(10)",
		-1:                             "IEEE754Class(-1)",
	} {
		if got := c.String(); got != want {
			t.Errorf("IEEE754Class(%d).String() = %q, want %q", int(c), got, want)
		}
	}
}

// TestIsCanonicalEncoding128 checks IsCanonicalBID128 and IsCanonicalDPD128
// on canonical and non-canonical encodings of each kind (IEEE 754-2019
// clause 3.5.2), each in its own encoding.
func TestIsCanonicalEncoding128(t *testing.T) {
	type enc struct {
		name   string
		hi, lo uint64
		want   bool
	}
	bits := func(f func(decimal128) (uint64, uint64), x decimal128) (uint64, uint64) { return f(x) }
	oneHi, oneLo := bits(decimal.BID128bits, 1)
	nanHi, nanLo := bits(decimal.BID128bits, decimal.NaNWithPayload(decimal128(42)))
	infHi, _ := bits(decimal.BID128bits, decimal.Inf[decimal128](1))
	const (
		p33Hi, p33Lo = 0x0000314dc6448d93, 0x38c15b0a00000000 // 10^33
		p34Hi, p34Lo = 0x0001ed09bead87c0, 0x378d8e6400000000 // 10^34
	)
	for _, tt := range []enc{
		{"one", oneHi, oneLo, true},
		{"NaN payload 42", nanHi, nanLo, true},
		{"infinity", infHi, 0, true},
		{"infinity, trailing field set", infHi, 1, false},
		{"infinity, G5 set", infHi | 1<<57, 0, false},
		{"NaN, G6 set", nanHi | 1<<56, nanLo, false},
		{"NaN payload 10^33 - 1", 0x7c00000000000000 | p33Hi, p33Lo - 1, true},
		{"NaN payload 10^33", 0x7c00000000000000 | p33Hi, p33Lo, false},
		{"significand 10^34 - 1", 0x3040000000000000 | p34Hi, p34Lo - 1, true},
		{"significand 10^34", 0x3040000000000000 | p34Hi, p34Lo, false},
		{"second form", 0x6000000000000000, 7, false},
	} {
		if got := decimal.IsCanonicalBID128(tt.hi, tt.lo); got != tt.want {
			t.Errorf("BID %s: IsCanonicalBID128(%#x, %#x) = %v, want %v", tt.name, tt.hi, tt.lo, got, tt.want)
		}
	}

	oneHi, oneLo = bits(decimal.DPD128bits, 1)
	nanHi, nanLo = bits(decimal.DPD128bits, decimal.NaNWithPayload(decimal128(42)))
	infHi, _ = bits(decimal.DPD128bits, decimal.Inf[decimal128](1))
	maxHi, maxLo := bits(decimal.DPD128bits, decimal.MaxDecimal128)
	for _, tt := range []enc{
		{"one", oneHi, oneLo, true},
		{"max", maxHi, maxLo, true},
		{"NaN payload 42", nanHi, nanLo, true},
		{"infinity", infHi, 0, true},
		{"infinity, trailing field set", infHi, 1, false},
		{"infinity, G5 set", infHi | 1<<57, 0, false},
		{"NaN, G6 set", nanHi | 1<<56, nanLo, false},
		{"declet 0x06e (canonical 00x11x111x)", oneHi, oneLo&^0x3ff | 0x06e, true},
		{"declet 0x16e (01x11x111x)", oneHi, oneLo&^0x3ff | 0x16e, false},
		{"declet 0x26e (10x11x111x)", oneHi, oneLo&^0x3ff | 0x26e, false},
		{"declet 0x3ff (11x11x111x), the top one", oneHi | 0x3ff<<36, oneLo, false},
		{"NaN payload declet 0x16e", nanHi, nanLo&^0x3ff | 0x16e, false},
	} {
		if got := decimal.IsCanonicalDPD128(tt.hi, tt.lo); got != tt.want {
			t.Errorf("DPD %s: IsCanonicalDPD128(%#x, %#x) = %v, want %v", tt.name, tt.hi, tt.lo, got, tt.want)
		}
	}
}
