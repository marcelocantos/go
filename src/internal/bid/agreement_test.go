// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// The reference model (ref_test.go, refops_test.go), the conformance suite
// (testdata) and the published vectors (vectors_test.go) were written
// independently of each other and of the kernel. TestConformanceReference
// runs the whole conformance suite against the reference model, so that
// the oracle is validated against expectations it did not produce, and a
// later edit to the oracle or to the suite that breaks their agreement
// fails here. The published vectors are run against the reference by
// TestPublishedVectors with -bid.vectors=reference.

import (
	"internal/bid"
	"testing"
)

// opsFromImpl is the conformance table of an implementation seen through
// the width-generic impl interface. A decimal64 travels in the Lo word of
// a bits value, as in impl.
func opsFromImpl(im impl) conformanceOps {
	d64, d128 := dec64, dec128
	return conformanceOps{
		Add64:  func(x, y uint64, m bid.Mode) uint64 { return im.add(d64, lo(x), lo(y), m).Lo },
		Sub64:  func(x, y uint64, m bid.Mode) uint64 { return im.sub(d64, lo(x), lo(y), m).Lo },
		Mul64:  func(x, y uint64, m bid.Mode) uint64 { return im.mul(d64, lo(x), lo(y), m).Lo },
		Div64:  func(x, y uint64, m bid.Mode) uint64 { return im.div(d64, lo(x), lo(y), m).Lo },
		Add128: func(x, y bid.Uint128, m bid.Mode) bid.Uint128 { return im.add(d128, x, y, m) },
		Sub128: func(x, y bid.Uint128, m bid.Mode) bid.Uint128 { return im.sub(d128, x, y, m) },
		Mul128: func(x, y bid.Uint128, m bid.Mode) bid.Uint128 { return im.mul(d128, x, y, m) },
		Div128: func(x, y bid.Uint128, m bid.Mode) bid.Uint128 { return im.div(d128, x, y, m) },
		FMA64:  func(x, y, z uint64, m bid.Mode) uint64 { return im.fma(d64, lo(x), lo(y), lo(z), m).Lo },
		FMA128: func(x, y, z bid.Uint128, m bid.Mode) bid.Uint128 { return im.fma(d128, x, y, z, m) },

		Rem64:  func(x, y uint64) uint64 { return im.rem(d64, lo(x), lo(y)).Lo },
		Mod64:  func(x, y uint64) uint64 { return im.mod(d64, lo(x), lo(y)).Lo },
		Rem128: func(x, y bid.Uint128) bid.Uint128 { return im.rem(d128, x, y) },
		Mod128: func(x, y bid.Uint128) bid.Uint128 { return im.mod(d128, x, y) },

		RoundToIntegral64:  func(x uint64, m bid.Mode) uint64 { return im.roundToIntegral(d64, lo(x), m).Lo },
		RoundToIntegral128: func(x bid.Uint128, m bid.Mode) bid.Uint128 { return im.roundToIntegral(d128, x, m) },
		Quantize64:         func(x, y uint64, m bid.Mode) uint64 { return im.quantize(d64, lo(x), lo(y), m).Lo },
		Quantize128:        func(x, y bid.Uint128, m bid.Mode) bid.Uint128 { return im.quantize(d128, x, y, m) },
		Reduce64:           func(x uint64) uint64 { return im.reduce(d64, lo(x)).Lo },
		Reduce128:          func(x bid.Uint128) bid.Uint128 { return im.reduce(d128, x) },
		SameQuantum64:      func(x, y uint64) bool { return im.sameQuantum(d64, lo(x), lo(y)) },
		SameQuantum128:     func(x, y bid.Uint128) bool { return im.sameQuantum(d128, x, y) },
		Min64:              func(x, y uint64) uint64 { return im.min(d64, lo(x), lo(y)).Lo },
		Max64:              func(x, y uint64) uint64 { return im.max(d64, lo(x), lo(y)).Lo },
		Min128:             func(x, y bid.Uint128) bid.Uint128 { return im.min(d128, x, y) },
		Max128:             func(x, y bid.Uint128) bid.Uint128 { return im.max(d128, x, y) },

		Round64: func(neg bool, c uint64, exp int, sticky bool, m bid.Mode) (uint64, bid.Status) {
			x, st := im.round(d64, neg, bid.Uint128{Lo: c}, exp, sticky, m)
			return x.Lo, st
		},
		Round128: func(neg bool, c bid.Uint128, exp int, sticky bool, m bid.Mode) (bid.Uint128, bid.Status) {
			return im.round(d128, neg, c, exp, sticky, m)
		},

		Equal64:       func(x, y uint64) bool { return im.equal(d64, lo(x), lo(y)) },
		Less64:        func(x, y uint64) bool { return im.less(d64, lo(x), lo(y)) },
		LessEqual64:   func(x, y uint64) bool { return im.lessEqual(d64, lo(x), lo(y)) },
		TotalOrder64:  func(x, y uint64) bool { return im.totalOrder(d64, lo(x), lo(y)) },
		Equal128:      func(x, y bid.Uint128) bool { return im.equal(d128, x, y) },
		Less128:       func(x, y bid.Uint128) bool { return im.less(d128, x, y) },
		LessEqual128:  func(x, y bid.Uint128) bool { return im.lessEqual(d128, x, y) },
		TotalOrder128: func(x, y bid.Uint128) bool { return im.totalOrder(d128, x, y) },
		Compare64:     func(x, y uint64) (int, bool) { return im.compare(d64, lo(x), lo(y)) },
		Compare128:    func(x, y bid.Uint128) (int, bool) { return im.compare(d128, x, y) },
		Key64:         func(x uint64) bid.Key { return im.key(d64, lo(x)) },
		Key128:        func(x bid.Uint128) bid.Key { return im.key(d128, x) },

		Widen:         func(x uint64) bid.Uint128 { return im.widen(lo(x)) },
		Narrow:        func(x bid.Uint128, m bid.Mode) uint64 { return im.narrow(x, m).Lo },
		Int64To64:     func(x int64, m bid.Mode) uint64 { return im.int64To(d64, x, m).Lo },
		Int64To128:    func(x int64) bid.Uint128 { return im.int64To(d128, x, bid.ToNearestEven) },
		Uint64To64:    func(x uint64, m bid.Mode) uint64 { return im.uint64To(d64, x, m).Lo },
		Uint64To128:   func(x uint64) bid.Uint128 { return im.uint64To(d128, x, bid.ToNearestEven) },
		Int64From64:   func(x uint64, m bid.Mode) (int64, bid.Status) { return im.int64From(d64, lo(x), m) },
		Int64From128:  func(x bid.Uint128, m bid.Mode) (int64, bid.Status) { return im.int64From(d128, x, m) },
		Uint64From64:  func(x uint64, m bid.Mode) (uint64, bid.Status) { return im.uint64From(d64, lo(x), m) },
		Uint64From128: func(x bid.Uint128, m bid.Mode) (uint64, bid.Status) { return im.uint64From(d128, x, m) },

		Float64To64:    func(f float64, m bid.Mode) uint64 { return im.float64To(d64, f, m).Lo },
		Float64To128:   func(f float64, m bid.Mode) bid.Uint128 { return im.float64To(d128, f, m) },
		Float32To64:    func(f float32, m bid.Mode) uint64 { return im.float32To(d64, f, m).Lo },
		Float32To128:   func(f float32, m bid.Mode) bid.Uint128 { return im.float32To(d128, f, m) },
		Float64From64:  func(x uint64, m bid.Mode) float64 { return im.float64From(d64, lo(x), m) },
		Float64From128: func(x bid.Uint128, m bid.Mode) float64 { return im.float64From(d128, x, m) },
		Float32From64:  func(x uint64, m bid.Mode) float32 { return im.float32From(d64, lo(x), m) },
		Float32From128: func(x bid.Uint128, m bid.Mode) float32 { return im.float32From(d128, x, m) },

		Append64:  func(dst []byte, x uint64) []byte { return append(dst, im.appendTo(d64, lo(x))...) },
		Append128: func(dst []byte, x bid.Uint128) []byte { return append(dst, im.appendTo(d128, x)...) },
		Parse64: func(s string, m bid.Mode) (uint64, bid.Status) {
			x, st := im.parse(d64, s, m)
			return x.Lo, st
		},
		Parse128: func(s string, m bid.Mode) (bid.Uint128, bid.Status) { return im.parse(d128, s, m) },

		Unpack64: func(x uint64) (bid.Kind, bool, uint64, int) {
			k, neg, c, exp := im.unpack(d64, lo(x))
			return k, neg, c.Lo, exp
		},
		Unpack128: func(x bid.Uint128) (bid.Kind, bool, bid.Uint128, int) {
			return im.unpack(d128, x)
		},
		Pack64: func(neg bool, c uint64, exp int) (uint64, bool) {
			x, ok := im.pack(d64, neg, bid.Uint128{Lo: c}, exp)
			return x.Lo, ok
		},
		Pack128: func(neg bool, c bid.Uint128, exp int) (bid.Uint128, bool) { return im.pack(d128, neg, c, exp) },

		IsNaN64:        func(x uint64) bool { return im.isNaN(d64, lo(x)) },
		IsSignaling64:  func(x uint64) bool { return im.isSignaling(d64, lo(x)) },
		IsInf64:        func(x uint64) bool { return im.isInf(d64, lo(x)) },
		IsFinite64:     func(x uint64) bool { return im.isFinite(d64, lo(x)) },
		IsZero64:       func(x uint64) bool { return im.isZero(d64, lo(x)) },
		IsSubnormal64:  func(x uint64) bool { return im.isSubnormal(d64, lo(x)) },
		IsCanonical64:  func(x uint64) bool { return im.isCanonical(d64, lo(x)) },
		Signbit64:      func(x uint64) bool { return im.signbit(d64, lo(x)) },
		IsNaN128:       func(x bid.Uint128) bool { return im.isNaN(d128, x) },
		IsSignaling128: func(x bid.Uint128) bool { return im.isSignaling(d128, x) },
		IsInf128:       func(x bid.Uint128) bool { return im.isInf(d128, x) },
		IsFinite128:    func(x bid.Uint128) bool { return im.isFinite(d128, x) },
		IsZero128:      func(x bid.Uint128) bool { return im.isZero(d128, x) },
		IsSubnormal128: func(x bid.Uint128) bool { return im.isSubnormal(d128, x) },
		IsCanonical128: func(x bid.Uint128) bool { return im.isCanonical(d128, x) },
		Signbit128:     func(x bid.Uint128) bool { return im.signbit(d128, x) },

		Neg64:       func(x uint64) uint64 { return im.neg(d64, lo(x)).Lo },
		Abs64:       func(x uint64) uint64 { return im.abs(d64, lo(x)).Lo },
		Neg128:      func(x bid.Uint128) bid.Uint128 { return im.neg(d128, x) },
		Abs128:      func(x bid.Uint128) bid.Uint128 { return im.abs(d128, x) },
		CopySign64:  func(x, s uint64) uint64 { return im.copySign(d64, lo(x), lo(s)).Lo },
		CopySign128: func(x, s bid.Uint128) bid.Uint128 { return im.copySign(d128, x, s) },
	}
}

// referenceOps is the reference model as a conformance table.
var referenceOps = opsFromImpl(reference{})

// agreementSkips lists the conformance vectors, by position (file:line),
// on which the reference model and the suite disagree and the disagreement
// waits on a decision about the contract (doc.go is ambiguous or wrong
// there), each with the reason. A listed vector is reported and counted,
// and fails if it passes. The suite declares nothing inapplicable and the
// reference model implements every operation, so every other vector must
// run and match.
var agreementSkips = map[string]string{}

// TestConformanceReference runs the conformance suite against the
// reference model. Every vector must run and match, except those named in
// agreementSkips.
func TestConformanceReference(t *testing.T) {
	vectors := loadConformance(t)
	if t.Failed() {
		t.FailNow()
	}
	runConformance(t, vectors, &referenceOps, conformanceRun{strict: true, known: agreementSkips})
}
