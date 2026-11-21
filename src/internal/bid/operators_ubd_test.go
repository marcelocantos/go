// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"internal/bid"
	dec "math/decimal"
	"testing"
	"unsafe"
)

// The compiled decimal128 operators on raw UBD memory. TestOperators builds
// its operands with BID128frombits, which converts to canonical UBD, so the
// well-formed patterns that only memory can hold (a NaN payload out of
// range, unused bits in a special) never
// reach the inline paths there. TestOperatorsRawUBD gives the compiled
// operators every pair of ubdPool128, as stored words, and checks each
// result against the BID function on the operands as the UBD kernel
// contract reads them (see ubd.go): the inline paths must agree with the
// kernel on every well-formed pattern, and must produce no malformed one.

// rawUBD128 returns the decimal128 whose memory holds the UBD words x, low
// word first.
func rawUBD128(x bid.Uint128) decimal128 {
	w := [2]uint64{x.Lo, x.Hi}
	return *(*decimal128)(unsafe.Pointer(&w))
}

// wordsUBD128 returns the UBD words in the memory of d.
func wordsUBD128(d decimal128) bid.Uint128 {
	w := *(*[2]uint64)(unsafe.Pointer(&d))
	return bid.Uint128{Hi: w[1], Lo: w[0]}
}

//go:noinline
func opRound128(x decimal128) decimal128 { return dec.Round(x) }

//go:noinline
func opRoundToEven128(x decimal128) decimal128 { return dec.RoundToEven(x) }

//go:noinline
func opFloor128(x decimal128) decimal128 { return dec.Floor(x) }

func TestOperatorsRawUBD(t *testing.T) {
	if !bid.NativeUBD128 {
		t.Skip("decimal128 is not stored as UBD on this target")
	}
	// The memory form round-trips through the test's conversions.
	if one := decimal128(1); wordsUBD128(one) != bid.UBDFromBID128(bid.Uint128{Hi: bid.Zero128Hi, Lo: 1}) {
		t.Fatalf("decimal128(1) is stored as %#x, not UBD low word first", wordsUBD128(one))
	}

	pool := ubdPool128(t)
	zero := bid.Uint128{Hi: bid.Zero128Hi}
	errs := 0
	fail := func(format string, args ...any) {
		t.Helper()
		if errs++; errs <= 20 {
			t.Errorf(format, args...)
		} else if errs == 21 {
			t.Errorf("too many errors")
		}
	}
	type binary = func(x, y decimal128) decimal128
	ops := []struct {
		name string
		f    binary
		ref  func(x, y bid.Uint128, mode bid.Mode) bid.Uint128
	}{
		{"+", opAdd128, bid.Add128},
		{"-", opSub128, bid.Sub128},
		{"*", opMul128, bid.Mul128},
		{"/", opDiv128, bid.Div128},
	}
	unary := []struct {
		name string
		f    func(x decimal128) decimal128
		mode bid.Mode
	}{
		{"Round", opRound128, bid.ToNearestAway},
		{"RoundToEven", opRoundToEven128, bid.ToNearestEven},
		{"Floor", opFloor128, bid.TowardNegative},
	}
	cmp := func(x, y bid.Uint128) [6]bool {
		c, ok := bid.Compare128(x, y)
		return [6]bool{ok && c == 0, !ok || c != 0, ok && c < 0, ok && c <= 0, ok && c > 0, ok && c >= 0}
	}

	for _, x := range pool {
		dx, bx := rawUBD128(x), bid.BIDFromUBD128(x)
		if got, want := opZero128(dx), cmp(bx, zero); [6]bool(got[4:10]) != [6]bool{want[2], want[3], want[4], want[5], want[4], want[5]} || got[0] != want[0] || got[1] != want[1] {
			fail("zero comparisons of %#x: got %v; want ==0 %v, <0 %v, >0 %v", x, got, want[0], want[2], want[4])
		}
		for _, u := range unary {
			got, want := wordsUBD128(u.f(dx)), bid.UBDFromBID128(bid.RoundToIntegral128(bx, u.mode))
			mustWellFormed128(t, u.name, got)
			if got != want {
				fail("%s(%#x) = %#x, want %#x", u.name, x, got, want)
			}
		}
		for _, y := range pool {
			dy, by := rawUBD128(y), bid.BIDFromUBD128(y)
			if got, want := opCmp128(dx, dy), cmp(bx, by); got != want {
				fail("comparisons of %#x and %#x (==, !=, <, <=, >, >=): got %v, want %v", x, y, got, want)
			}
			for _, op := range ops {
				got, want := wordsUBD128(op.f(dx, dy)), bid.UBDFromBID128(op.ref(bx, by, bid.ToNearestEven))
				mustWellFormed128(t, op.name, got)
				if got != want {
					fail("%#x %s %#x = %#x, want %#x", x, op.name, y, got, want)
				}
			}
			for _, m := range []dec.RoundingMode{dec.ToNearestEven, dec.ToZero} {
				got := wordsUBD128(compiledQuant128(dx, dy, m))
				want := bid.UBDFromBID128(bid.Quantize128(bx, by, bid.Mode(m)))
				mustWellFormed128(t, "QuantizeMode", got)
				if got != want {
					fail("QuantizeMode(%#x, %#x, %d) = %#x, want %#x", x, y, m, got, want)
				}
			}
		}
	}
}
