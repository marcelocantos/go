// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/rand/v2"
	"testing"
)

// TestDPDDecletCanonical checks the bit rule for the 24 non-canonical
// declets against the tables: a declet is canonical exactly when encoding
// its value gives it back.
func TestDPDDecletCanonical(t *testing.T) {
	n := 0
	for d := range uint64(1024) {
		want := uint64(bid.Bin2DPD[bid.DPD2Bin[d]]) == d
		if got := bid.DPDDecletCanonical(d); got != want {
			t.Errorf("DPDDecletCanonical(%#03x) = %v, want %v", d, got, want)
		}
		if want {
			n++
		}
	}
	if n != 1000 {
		t.Errorf("%d canonical declets, want 1000", n)
	}
}

// TestIsCanonical128Encodings checks IsCanonical128 (BID) and
// IsCanonicalDPD128 against the round trip through the package's codecs,
// which keep a canonical encoding bit for bit and canonicalize every other
// one: BID through UBD and back, DPD through BID and back. The patterns are
// random 128-bit words, the same with the combination field forced to each
// class's tag, and canonical encodings of random values with one bit
// flipped.
func TestIsCanonical128Encodings(t *testing.T) {
	r := rand.New(rand.NewPCG(41, 3520))
	var patterns []bid.Uint128
	for range 100000 {
		x := bid.Uint128{Hi: r.Uint64(), Lo: r.Uint64()}
		patterns = append(patterns, x)
		for _, tag := range []uint64{0x1E, 0x1F} { // infinity, NaN
			y := x
			y.Hi = y.Hi&^(0x1F<<58) | tag<<58
			patterns = append(patterns, y)
			// The same with the reserved bits clear, which reaches
			// the canonical specials.
			y.Hi &= 1<<63 | 0x3F<<57 | (1<<46 - 1)
			patterns = append(patterns, y)
			y.Hi &^= 1<<46 - 1
			y.Lo = 0
			patterns = append(patterns, y)
		}
		c, ok := bid.Pack128(r.IntN(2) == 0, bid.Uint128{Hi: r.Uint64() >> 16, Lo: r.Uint64()}, bid.MinExp128+r.IntN(bid.MaxExp128-bid.MinExp128+1))
		if ok {
			patterns = append(patterns, c, bid.DPDFromBID128(c))
			bit := r.IntN(128)
			for _, p := range []bid.Uint128{c, bid.DPDFromBID128(c)} {
				if bit < 64 {
					p.Lo ^= 1 << bit
				} else {
					p.Hi ^= 1 << (bit - 64)
				}
				patterns = append(patterns, p)
			}
		}
	}

	var nBID, nDPD int
	for _, x := range patterns {
		wantBID := bid.BIDFromUBDAny128(bid.UBDFromBID128(x)) == x
		if got := bid.IsCanonical128(x); got != wantBID {
			t.Errorf("IsCanonical128(%#x) = %v, but the round trip says %v", x, got, wantBID)
		}
		wantDPD := bid.DPDFromBID128(bid.BIDFromDPD128(x)) == x
		if got := bid.IsCanonicalDPD128(x); got != wantDPD {
			t.Errorf("IsCanonicalDPD128(%#x) = %v, but the round trip says %v", x, got, wantDPD)
		}
		if wantBID {
			nBID++
		}
		if wantDPD {
			nDPD++
		}
	}
	t.Logf("%d patterns: %d canonical BID, %d canonical DPD", len(patterns), nBID, nDPD)
}

// TestIsCanonicalDPD128Declets puts each of the 1024 declets at each of the
// eleven positions of the trailing field of a finite number and of a NaN,
// the rest zero, and checks that IsCanonicalDPD128 is true exactly for the
// canonical ones.
func TestIsCanonicalDPD128Declets(t *testing.T) {
	const (
		finiteHi = 0x2208000000000000 // 0E+0
		nanHi    = 0x7c00000000000000
	)
	for k := range 11 {
		for d := range uint64(1024) {
			for _, head := range []uint64{finiteHi, nanHi} {
				x := bid.Uint128{Hi: head}
				if s := 10 * k; s < 64 {
					x.Lo = d << s
					if s > 54 {
						x.Hi |= d >> (64 - s)
					}
				} else {
					x.Hi |= d << (s - 64)
				}
				want := uint64(bid.Bin2DPD[bid.DPD2Bin[d]]) == d
				if got := bid.IsCanonicalDPD128(x); got != want {
					t.Errorf("IsCanonicalDPD128(%#x) = %v, want %v (declet %d = %#03x)", x, got, want, k, d)
				}
			}
		}
	}
}
