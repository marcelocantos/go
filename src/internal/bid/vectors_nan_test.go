// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// NaN operands and results of the published vectors, which the runner in
// vectors_test.go compares bit for bit, payload and sign included. This
// file reads the NaN words of the GDA test-case format with their payloads, and
// names the published vectors whose NaN results differ from the contract
// in a documented way. Each excuse is narrow: it applies only when the
// target returned exactly what the contract requires (computed here from
// the operands with the reference model, not taken from the target) and
// the vector expects exactly what the named rule of the other library
// gives. Every other NaN mismatch fails.

import (
	"math"
	"math/big"
	"regexp"
	"testing"
)

// vecNaNText matches a NaN word of the GDA test-case format: an optional sign,
// NaN or sNaN in any case, and an optional payload of decimal digits.
var vecNaNText = regexp.MustCompile(`(?i)^([+-]?)(s?)nan([0-9]*)$`)

// vecNaN returns the encoding of the NaN word s in format f, with its sign,
// its signaling bit and its payload, and ok = true; or ok = false when s is
// not a NaN word or its payload has more than prec-1 digits, which the
// format cannot hold.
func vecNaN(f *format, s string) (x bits, ok bool) {
	m := vecNaNText.FindStringSubmatch(s)
	if m == nil {
		return bits{}, false
	}
	payload := new(big.Int)
	if m[3] != "" {
		payload.SetString(m[3], 10)
	}
	if payload.Cmp(pow10(f.prec-1)) >= 0 {
		return bits{}, false
	}
	x = f.encode(value{kind: notANumber, neg: m[1] == "-", coeff: payload})
	if m[2] != "" {
		if f.width == 64 {
			x.Lo |= 1 << 57
		} else {
			x.Hi |= 1 << 57
		}
	}
	return x, true
}

// The NaN divergences of the published vectors from the contract. The
// first two are disputes between the two published suites, which the
// contract settles for GDA and for float64 on arm64; the rest are
// conversions whose NaN payloads the contract leaves alone (see "NaN
// results" in the package documentation).
const (
	skipNaNChoice        = "dispute: of several NaN operands Intel propagates the first (z first for fma), the contract the first signaling one (as GDA and float64 on arm64 do)"
	skipNaNScaled        = "dispute: Intel scales a NaN payload by 10^18 between decimal64 and decimal128; the contract keeps its integer value"
	skipNaNBinary        = "contract: conversion between binary and decimal floating point gives the canonical NaN (payloads across radices are out of scope)"
	skipNaNParse         = "contract: Parse reads every NaN as the canonical quiet NaN (sign, signaling and payload in the character form are out of scope)"
	skipNaNText          = "contract: the character form of every NaN is NaN (sign, signaling and payload in the character form are out of scope)"
	skipNaNPayloadSyntax = "contract: Parse rejects a NaN payload with Syntax (payloads in the character form are out of scope)"
	skipNaNMalformed     = "contract: a string outside the grammar gives the canonical NaN with Syntax (Intel reads it as another NaN)"
	skipNaNTotalOrder    = "contract: TotalOrder ignores the signaling bit and the payload of NaNs of one sign"
	skipGDAAbsNaN        = "op: GDA abs is arithmetic on a NaN (made quiet, sign kept); the contract's Abs is IEEE 754 abs, which copies the bits with the sign clear"
	noteUnorderedNaN     = "result matches; Compare reports unordered and returns no NaN, so the NaN's payload is not compared"
)

// gdaAbsDivergence reports skipGDAAbsNaN for a GDA abs vector of the NaN x
// when the target returned x with its sign bit clear (the contract's Abs)
// and the vector expects x made quiet with its sign (GDA's abs).
func gdaAbsDivergence(f *format, x, want, got bits) string {
	v := f.decode(x)
	if v.kind == notANumber && got == vecRef.abs(f, x) && want == f.encode(v) {
		return skipGDAAbsNaN
	}
	return ""
}

// totalOrderNaNDivergence reports skipNaNTotalOrder for a comparetotal
// whose operands x and y (made absolute for comparetotmag) are NaNs of one
// sign, which the contract orders both ways, and whose target answered 0.
func totalOrderNaNDivergence(f *format, x, y bits, got string) string {
	a, b := f.decode(x), f.decode(y)
	if a.kind == notANumber && b.kind == notANumber && a.neg == b.neg && got == "0" {
		return skipNaNTotalOrder
	}
	return ""
}

// payloadOf returns the sign and payload of the NaN x of format f, with ok
// = false when x is not a NaN. A non-canonical payload reads as zero.
func payloadOf(f *format, x bits) (neg bool, payload *big.Int, ok bool) {
	v := f.decode(x)
	if v.kind != notANumber {
		return false, nil, false
	}
	return v.neg, v.coeff, true
}

// nanChoiceDivergence reports, for a vector of an operation of the
// operands a that failed, whether it is the dispute skipNaNChoice: at
// least two operands are NaNs, the target returned the NaN the contract
// propagates (the first signaling NaN, else the first NaN, made quiet),
// and the vector expects another NaN operand made quiet instead.
func nanChoiceDivergence(f *format, a []bits, want, got bits) string {
	vs := make([]value, len(a))
	n := 0
	for i, x := range a {
		if vs[i] = f.decode(x); vs[i].kind == notANumber {
			n++
		}
	}
	if n < 2 {
		return ""
	}
	if contract := f.encode(nanOf(vs...)); got != contract || want == contract {
		return ""
	}
	for _, v := range vs {
		if v.kind == notANumber && f.encode(v) == want {
			return skipNaNChoice
		}
	}
	return ""
}

// nanScaleDivergence reports whether a NaN result that failed is the
// dispute skipNaNScaled: from the operand payload p (already in the
// format of the result if widened), Intel expects p × 10^18 where the
// contract gives p, or, narrowing the decimal128 operand x, Intel expects
// p / 10^18 where the contract gives 0 because p does not fit decimal64.
// got must be the contract's result exactly. from is the operand's format
// and x the operand for a narrowing; for any other operation from is nil.
func nanScaleDivergence(f *format, from *format, x, want, got bits) string {
	wneg, wp, ok1 := payloadOf(f, want)
	gneg, gp, ok2 := payloadOf(f, got)
	if !ok1 || !ok2 || wneg != gneg || got != f.encode(value{kind: notANumber, neg: gneg, coeff: gp}) {
		return ""
	}
	scale := pow10(18)
	if from == dec128 && f == dec64 {
		xneg, xp, ok := payloadOf(from, x)
		if ok && xneg == gneg && gp.Sign() == 0 && xp.Cmp(pow10(f.prec-1)) >= 0 &&
			new(big.Int).Mul(wp, scale).Cmp(xp) == 0 {
			return skipNaNScaled
		}
		return ""
	}
	if f == dec128 && gp.Sign() != 0 && new(big.Int).Mul(gp, scale).Cmp(wp) == 0 {
		return skipNaNScaled
	}
	return ""
}

// canonicalNaNDivergence reports reason when the vector expects a NaN other
// than the canonical quiet NaN and the target returned the canonical NaN of
// format f: the out-of-scope conversions skipNaNBinary and skipNaNParse.
func canonicalNaNDivergence(f *format, want, got bits, reason string) string {
	canon := f.encode(nan)
	if got == canon && want != canon && f.decode(want).kind == notANumber {
		return reason
	}
	return ""
}

// binaryNaNDivergence is canonicalNaNDivergence for a binary result of n
// bits: the canonical binary NaN is the quiet NaN with the sign bit clear
// and a zero payload.
func binaryNaNDivergence(n int, want, got uint64) string {
	canon := uint64(0x7ff8000000000000)
	isNaN := func(b uint64) bool { return math.IsNaN(math.Float64frombits(b)) }
	if n == 32 {
		canon = 0x7fc00000
		isNaN = func(b uint64) bool { return math.IsNaN(float64(math.Float32frombits(uint32(b)))) }
	}
	if got == canon && want != canon && isNaN(want) {
		return skipNaNBinary
	}
	return ""
}

// textNaNDivergence reports skipNaNText when the vector expects a NaN word
// other than NaN and the target wrote NaN.
func textNaNDivergence(want, got string) string {
	if got == "NaN" && want != "NaN" && vecNaNText.MatchString(want) {
		return skipNaNText
	}
	return ""
}

// TestNaNDivergences checks that the NaN excuses of the published-vector
// runner apply exactly where documented and nowhere wider.
func TestNaNDivergences(t *testing.T) {
	n := func(f *format, s string) bits {
		x, ok := vecNaN(f, s)
		if !ok {
			t.Fatalf("vecNaN(%s, %q) failed", f.name, s)
		}
		return x
	}
	one := dec64.encode(value{kind: finite, coeff: big.NewInt(1)})

	// vecNaN.
	if x := n(dec64, "-sNaN123"); x.Lo != 0xfe0000000000007b {
		t.Errorf("vecNaN(-sNaN123) = %#x", x.Lo)
	}
	if x := n(dec128, "NaN"); x != dec128.encode(nan) {
		t.Errorf("vecNaN(NaN) = %#x", x)
	}
	if _, ok := vecNaN(dec64, "NaN1000000000000000"); ok {
		t.Errorf("vecNaN accepted a 16-digit decimal64 payload")
	}
	if _, ok := vecNaN(dec64, "1"); ok {
		t.Errorf("vecNaN accepted a number")
	}

	// nanChoiceDivergence: q, s are a quiet and a signaling NaN.
	q, s := n(dec64, "NaN1"), n(dec64, "sNaN2")
	qq, sq := n(dec64, "NaN1"), n(dec64, "NaN2") // q and s made quiet
	choice := []struct {
		name      string
		a         []bits
		want, got bits
		reason    string
	}{
		{"first NaN expected, signaling returned", []bits{q, s}, qq, sq, skipNaNChoice},
		{"z expected in fma", []bits{s, one, q}, qq, sq, skipNaNChoice},
		{"one NaN operand", []bits{q, one}, qq, sq, ""},
		{"target not the contract", []bits{q, s}, sq, qq, ""},
		{"expected a non-operand NaN", []bits{q, s}, n(dec64, "NaN3"), sq, ""},
		{"expected the canonical NaN", []bits{q, s}, dec64.encode(nan), sq, ""},
	}
	for _, c := range choice {
		if got := nanChoiceDivergence(dec64, c.a, c.want, c.got); got != c.reason {
			t.Errorf("nanChoiceDivergence %s: got %q, want %q", c.name, got, c.reason)
		}
	}

	// nanScaleDivergence.
	scaled := n(dec128, "-NaN7000000000000000000")
	scale := []struct {
		name         string
		f, from      *format
		x, want, got bits
		reason       string
	}{
		{"widened, scaled expected", dec128, nil, bits{}, scaled, n(dec128, "-NaN7"), skipNaNScaled},
		{"widened, sign differs", dec128, nil, bits{}, scaled, n(dec128, "NaN7"), ""},
		{"widened, other payload", dec128, nil, bits{}, scaled, n(dec128, "-NaN8"), ""},
		{"widened, zero payload", dec128, nil, bits{}, n(dec128, "NaN"), n(dec128, "NaN"), ""},
		{"narrowed, scaled down expected", dec64, dec128, n(dec128, "NaN5000000000000000000000"), n(dec64, "NaN5000"), n(dec64, "NaN"), skipNaNScaled},
		{"narrowed, payload fits", dec64, dec128, n(dec128, "NaN5000"), n(dec64, "NaN0"), n(dec64, "NaN5000"), ""},
		{"narrowed, target kept a payload", dec64, dec128, n(dec128, "NaN5000000000000000000000"), n(dec64, "NaN5000"), n(dec64, "NaN1"), ""},
	}
	for _, c := range scale {
		if got := nanScaleDivergence(c.f, c.from, c.x, c.want, c.got); got != c.reason {
			t.Errorf("nanScaleDivergence %s: got %q, want %q", c.name, got, c.reason)
		}
	}

	// canonicalNaNDivergence, binaryNaNDivergence, textNaNDivergence.
	canon := dec64.encode(nan)
	if got := canonicalNaNDivergence(dec64, n(dec64, "sNaN"), canon, skipNaNParse); got != skipNaNParse {
		t.Errorf("canonicalNaNDivergence(sNaN, canonical) = %q", got)
	}
	if got := canonicalNaNDivergence(dec64, n(dec64, "sNaN"), n(dec64, "NaN1"), skipNaNParse); got != "" {
		t.Errorf("canonicalNaNDivergence(sNaN, NaN1) = %q", got)
	}
	if got := canonicalNaNDivergence(dec64, one, canon, skipNaNParse); got != "" {
		t.Errorf("canonicalNaNDivergence(1, canonical) = %q", got)
	}
	if got := binaryNaNDivergence(64, 0x7ff8000000000001, 0x7ff8000000000000); got != skipNaNBinary {
		t.Errorf("binaryNaNDivergence(64) = %q", got)
	}
	if got := binaryNaNDivergence(32, 0xffc00000, 0x7fc00000); got != skipNaNBinary {
		t.Errorf("binaryNaNDivergence(32) = %q", got)
	}
	if got := binaryNaNDivergence(64, 0x7ff8000000000001, 0xfff8000000000000); got != "" {
		t.Errorf("binaryNaNDivergence(non-canonical got) = %q", got)
	}
	if got := textNaNDivergence("-NaN12", "NaN"); got != skipNaNText {
		t.Errorf("textNaNDivergence(-NaN12, NaN) = %q", got)
	}
	if got := textNaNDivergence("-NaN12", "-NaN"); got != "" {
		t.Errorf("textNaNDivergence(-NaN12, -NaN) = %q", got)
	}
	if got := textNaNDivergence("1", "NaN"); got != "" {
		t.Errorf("textNaNDivergence(1, NaN) = %q", got)
	}
}
