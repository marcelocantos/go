// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"testing"
)

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
