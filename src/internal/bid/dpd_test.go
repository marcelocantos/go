// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

// TestDeclets checks the declet tables: every integer below 1000 has a
// declet that decodes back to it, every one of the 1024 declets decodes to
// an integer below 1000, and exactly 24 declets are redundant (decode to an
// integer whose canonical declet is another one), as clause 3.5.2 says.
func TestDeclets(t *testing.T) {
	seen := map[uint16]bool{}
	for n := range 1000 {
		d := bid.Bin2DPD[n]
		if d > 1023 || bid.DPD2Bin[d] != uint16(n) {
			t.Errorf("declet %#03x of %d decodes to %d", d, n, bid.DPD2Bin[d])
		}
		if seen[d] {
			t.Errorf("declet %#03x is the declet of two integers", d)
		}
		seen[d] = true
	}
	redundant := 0
	for d := range 1024 {
		n := bid.DPD2Bin[d]
		if n > 999 {
			t.Errorf("declet %#03x decodes to %d", d, n)
			continue
		}
		if bid.Bin2DPD[n] != uint16(d) {
			redundant++
			// A redundant declet has digits 8 or 9 in all three places
			// (Table 3.3, the row with v w x s t = 1 1 1 1 1).
			if d&0x6E != 0x6E || n/100 < 8 || n/10%10 < 8 || n%10 < 8 {
				t.Errorf("redundant declet %#03x decodes to %d", d, n)
			}
		}
	}
	if redundant != 24 {
		t.Errorf("%d redundant declets, want 24", redundant)
	}
	// Spot checks, from Table 3.4.
	for _, c := range []struct {
		n int
		d uint16
	}{{0, 0}, {9, 0x009}, {10, 0x010}, {79, 0x079}, {750, 0x3D0}, {999, 0x0FF}, {888, 0x06E}} {
		if got := bid.Bin2DPD[c.n]; got != c.d {
			t.Errorf("declet of %d is %#03x, want %#03x", c.n, got, c.d)
		}
	}
}

func TestDPDZero(t *testing.T) {
	if got := bid.DPDFromBID64(bid.Zero64); got != bid.DPDZero64 {
		t.Errorf("DPD of 0E+0 is %#016x, want DPDZero64 %#016x", got, bid.DPDZero64)
	}
	if got := bid.DPDFromBID128(bid.Uint128{Hi: bid.Zero128Hi}); got != (bid.Uint128{Hi: bid.DPDZero128Hi}) {
		t.Errorf("DPD of 0E+0 is %#016x%016x, want DPDZero128Hi", got.Hi, got.Lo)
	}
}

// TestDPDRoundTrip checks that the conversions are inverses on canonical
// encodings of every kind, drawn from the oracle's operand generator, and
// that every DPD pattern, canonical or not, converts to a canonical BID
// encoding of the value it reads as.
func TestDPDRoundTrip(t *testing.T) {
	n := numCases()
	for _, f := range bothFormats() {
		g := newGen(f, "dpd")
		for i := range n {
			x := g.next(i)
			if f.Width() == 64 {
				c := bid.BIDFromDPD64(bid.DPDFromBID64(x.Lo))
				if want := canonical64(x.Lo); c != want {
					t.Fatalf("BID→DPD→BID of %#016x is %#016x, want %#016x", x.Lo, c, want)
				}
				d := g.r.Uint64()
				b := bid.BIDFromDPD64(d)
				if !bid.IsCanonical64(b) {
					t.Fatalf("BIDFromDPD64(%#016x) = %#016x is not canonical", d, b)
				}
				if bid.BIDFromDPD64(bid.DPDFromBID64(b)) != b {
					t.Fatalf("DPD %#016x: canonical form does not round-trip", d)
				}
			} else {
				c := bid.BIDFromDPD128(bid.DPDFromBID128(x))
				if want := canonical128(x); c != want {
					t.Fatalf("BID→DPD→BID of %#016x%016x is %#016x%016x, want %#016x%016x", x.Hi, x.Lo, c.Hi, c.Lo, want.Hi, want.Lo)
				}
				d := bid.Uint128{Hi: g.r.Uint64(), Lo: g.r.Uint64()}
				b := bid.BIDFromDPD128(d)
				if !bid.IsCanonical128(b) {
					t.Fatalf("BIDFromDPD128(%#016x%016x) = %#016x%016x is not canonical", d.Hi, d.Lo, b.Hi, b.Lo)
				}
				if bid.BIDFromDPD128(bid.DPDFromBID128(b)) != b {
					t.Fatalf("DPD %#016x%016x: canonical form does not round-trip", d.Hi, d.Lo)
				}
			}
		}
	}
}

// canonical64 returns the canonical BID encoding of the value x reads as
// (clause 3.5.2): what the DPD round trip must give.
func canonical64(x uint64) uint64 {
	kind, neg, c, e := bid.Unpack64(x)
	sign := x & bid.SignMask64
	switch kind {
	case bid.KindInf:
		return sign | bid.Inf64
	case bid.KindNaN:
		p := x & (1<<50 - 1)
		if p >= 1e15 {
			p = 0
		}
		return sign | x&(bid.NaN64|bid.SignalingBit64) | p
	}
	r, _ := bid.Pack64(neg, c, e)
	return r
}

func canonical128(x bid.Uint128) bid.Uint128 {
	kind, neg, c, e := bid.Unpack128(x)
	sign := x.Hi & bid.SignMask128Hi
	switch kind {
	case bid.KindInf:
		return bid.Uint128{Hi: sign | bid.Inf128Hi}
	case bid.KindNaN:
		p := new(big.Int).Lsh(new(big.Int).SetUint64(x.Hi&(1<<46-1)), 64)
		p.Or(p, new(big.Int).SetUint64(x.Lo))
		if p.Cmp(new(big.Int).Exp(big.NewInt(10), big.NewInt(33), nil)) >= 0 {
			p.SetInt64(0)
		}
		r := bidNaN(128, sign != 0, x.Hi&bid.SignalingBit128Hi != 0, p)
		return r
	}
	r, _ := bid.Pack128(neg, c, e)
	return r
}

// TestDPDNonCanonical fixes the treatment of the non-canonical encodings
// of both kinds at an explicit conversion between encodings. Raw math bits
// functions perform no such conversion and preserve every stored bit.
func TestDPDNonCanonical(t *testing.T) {
	// A BID coefficient above 10^16 - 1 reads as zero, with its sign and
	// exponent: 0x6C7386F26FC10000 has coefficient 10^16 and exponent 0.
	x := uint64(0x6C7386F26FC10000) | bid.SignMask64
	if k, _, c, e := bid.Unpack64(x); k != bid.KindFinite || c != 0 || e != 0 {
		t.Fatalf("test operand %#016x is not a non-canonical zero", x)
	}
	if got, want := bid.DPDFromBID64(x), bid.DPDZero64|bid.SignMask64; got != want {
		t.Errorf("DPDFromBID64(%#016x) = %#016x, want -0E+0 %#016x", x, got, want)
	}
	// A BID NaN payload of 10^15 or more becomes zero; reserved bits go.
	nan := bid.SNaN64 | 1e15 | 1<<55
	if got, want := bid.DPDFromBID64(nan), uint64(0x7e00000000000000); got != want {
		t.Errorf("DPDFromBID64(%#016x) = %#016x, want sNaN %#016x", nan, got, want)
	}
	// An infinity's reserved and trailing bits go.
	if got := bid.DPDFromBID64(bid.Inf64 | 0x0123456789); got != 0x7800000000000000 {
		t.Errorf("DPDFromBID64 of a non-canonical infinity = %#016x", got)
	}
	// Redundant declets read as their canonical twins: 0x3FF and 0x0FF are
	// both 999, so #22380000000003FF is 999 and canonicalizes to
	// #22380000000000FF.
	if got, want := bid.BIDFromDPD64(0x22380000000003FF), bid.BIDFromDPD64(0x22380000000000FF); got != want {
		t.Errorf("redundant declet: %#016x, want %#016x", got, want)
	}
	if got := bid.DPDFromBID64(bid.BIDFromDPD64(0x22380000000003FF)); got != 0x22380000000000FF {
		t.Errorf("redundant declet canonicalizes to %#016x", got)
	}
	// Reserved bits of a DPD NaN or infinity are dropped.
	if got := bid.BIDFromDPD64(0x7DFC000000000001); got != bid.NaN64|1 {
		t.Errorf("BIDFromDPD64(#7DFC000000000001) = %#016x, want NaN1", got)
	}
	if got := bid.BIDFromDPD64(0xFBFFFFFFFFFFFFFF); got != bid.Inf64|bid.SignMask64 {
		t.Errorf("BIDFromDPD64(#FBFFFFFFFFFFFFFF) = %#016x, want -Infinity", got)
	}
	// decimal128: a second-form BID encoding is always non-canonical.
	y := bid.Uint128{Hi: 0x6000000000000000 | 0x1820<<47, Lo: 5}
	if got, want := bid.DPDFromBID128(y), (bid.Uint128{Hi: bid.DPDZero128Hi}); got != want {
		t.Errorf("DPDFromBID128(second form) = %#016x%016x, want 0E+0", got.Hi, got.Lo)
	}
}

// bothFormats, newGen and the generator methods adapt the oracle's operand
// generator, which draws encodings of every kind in every regime, canonical
// and not.
func bothFormats() []*format { return both }

func (f *format) Width() int { return f.width }

func newGen(f *format, name string) *gen {
	return &gen{f: f, r: rand.New(rand.NewPCG(*seedFlag, streamSeed(name+f.name)))}
}

func (g *gen) next(i int) bits {
	g.regime = regime(i % int(numRegimes))
	return g.dec()
}
