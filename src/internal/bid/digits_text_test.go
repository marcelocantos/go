// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/rand/v2"
	"testing"
)

// TestUnpackDigits compares the unformatted parts with the independent decoder and
// big.Int decimal conversion, including raw noncanonical and special inputs.
func TestUnpackDigits(t *testing.T) {
	for _, f := range both {
		g := &gen{f: f, r: rand.New(rand.NewPCG(*seedFlag, streamSeed("digits-"+f.name)))}
		for i := range numCases() {
			g.regime = regime(i % int(numRegimes))
			x := bits{Lo: g.r.Uint64()}
			if f == dec128 {
				x.Hi = g.r.Uint64()
			}
			if i&1 != 0 {
				x = f.encode(g.finite())
			}
			v := f.decode(x)
			wantKind, want := bid.KindFinite, ""
			switch v.kind {
			case infinite:
				wantKind = bid.KindInf
			case notANumber:
				wantKind = bid.KindNaN
			default:
				if v.coeff.Sign() != 0 {
					want = v.coeff.String()
				}
			}
			var buf64 [bid.Precision64]byte
			var buf128 [bid.Precision128]byte
			for j := range buf64 {
				buf64[j] = '?'
			}
			for j := range buf128 {
				buf128[j] = '?'
			}
			var kind bid.Kind
			var neg bool
			var n, exp int
			var buf []byte
			if f == dec64 {
				kind, neg, n, exp = bid.UnpackDigits64(&buf64, x.Lo)
				buf = buf64[:]
			} else {
				kind, neg, n, exp = bid.UnpackDigits128(&buf128, x)
				buf = buf128[:]
			}
			if kind != wantKind || neg != v.neg || exp != v.exp || n < 0 || n > len(buf) {
				t.Fatalf("%s UnpackDigits(%x): got (%v, %t, %d, %d), want (%v, %t, %d, %d)", f.name, x, kind, neg, n, exp, wantKind, v.neg, len(want), v.exp)
			}
			if string(buf[:n]) != want {
				t.Fatalf("%s UnpackDigits(%x) = %q, want %q", f.name, x, buf[:n], want)
			}
			for _, c := range buf[n:] {
				if c != '?' {
					t.Fatalf("%s UnpackDigits(%x) overwrote the buffer beyond %d", f.name, x, n)
				}
			}
		}
	}
}
