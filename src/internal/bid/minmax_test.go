// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"testing"
)

func TestMinMaxAllocs(t *testing.T) {
	x64, _ := bid.Parse64("1.50", bid.ToNearestEven)
	y64, _ := bid.Parse64("-2", bid.ToNearestEven)
	x128, _ := bid.Parse128("1.50", bid.ToNearestEven)
	y128, _ := bid.Parse128("-2", bid.ToNearestEven)
	var r64 uint64
	var r128 bid.Uint128
	for _, tt := range []struct {
		name string
		f    func()
	}{
		{"Min64", func() { r64 = bid.Min64(x64, y64) }},
		{"Max64", func() { r64 = bid.Max64(x64, y64) }},
		{"Min128", func() { r128 = bid.Min128(x128, y128) }},
		{"Max128", func() { r128 = bid.Max128(x128, y128) }},
	} {
		if got := testing.AllocsPerRun(100, tt.f); got != 0 {
			t.Errorf("%s allocates %v times", tt.name, got)
		}
	}
	_, _ = r64, r128
}

// TestMinMaxNonCanonical checks that a returned operand is canonicalized: a
// BID64 second-form coefficient above 10^16-1 reads as a zero with its sign
// and exponent, and that zero is what comes back.
func TestMinMaxNonCanonical(t *testing.T) {
	const nonCanonical uint64 = 0x6c70000000000000 | (1<<51 - 1) // exponent 0
	one, _ := bid.Parse64("1", bid.ToNearestEven)
	zero, _ := bid.Parse64("0", bid.ToNearestEven)
	if got := bid.Min64(nonCanonical, one); got != zero {
		t.Errorf("Min64(non-canonical zero, 1) = %#x, want %#x", got, zero)
	}
	if got := bid.Max64(one, nonCanonical); got != one {
		t.Errorf("Max64(1, non-canonical zero) = %#x, want %#x", got, one)
	}
}
