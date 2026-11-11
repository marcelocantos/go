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

// TestLong128 checks the long form of decimal128 against the reference:
// IsLong128 accepts exactly the finite canonical encodings with a
// coefficient of at least 10^8, and for those PutLong128 writes the
// reference's character form and no byte after it. The coefficients are the
// bounds of every length from 9 digits to 34, patterns that show a digit
// out of place (all nines, one digit set, a ramp) and a random sample of
// each length; the exponents put the point at every place inside the
// coefficient and beyond each end, cross both thresholds of exponential
// notation, and reach the limits of the format, where the exponent has
// four digits. Each sign.
func TestLong128(t *testing.T) {
	f := dec128
	r := rand.New(rand.NewPCG(*seedFlag, streamSeed("long128")))
	ten := big.NewInt(10)
	pow := func(k int) *big.Int { return new(big.Int).Exp(ten, big.NewInt(int64(k)), nil) }
	var coeffs []*big.Int
	add := func(c *big.Int) { coeffs = append(coeffs, c) }
	one := big.NewInt(1)
	for n := 9; n <= 34; n++ {
		lo, hi := pow(n-1), pow(n) // n digits: lo <= c < hi
		add(lo)
		add(new(big.Int).Add(lo, one))
		add(new(big.Int).Sub(hi, one))
		add(new(big.Int).Sub(hi, big.NewInt(2)))
		add(new(big.Int).Rsh(new(big.Int).Add(lo, hi), 1))
		// One digit set at each place, under a leading one.
		for i := 0; i < n-1; i++ {
			add(new(big.Int).Add(lo, new(big.Int).Mul(big.NewInt(int64(1+i%9)), pow(i))))
		}
		// A ramp of digits, which shows a lane or a byte out of order.
		ramp := new(big.Int)
		for i := 0; i < n; i++ {
			ramp.Mul(ramp, ten)
			ramp.Add(ramp, big.NewInt(int64(1+i%9)))
		}
		add(ramp)
		span := new(big.Int).Sub(hi, lo)
		for range 60 {
			// A value below span from 128 random bits: uniform enough here.
			v := new(big.Int).SetUint64(r.Uint64())
			v.Lsh(v, 64).Or(v, new(big.Int).SetUint64(r.Uint64()))
			add(new(big.Int).Add(lo, v.Mod(v, span)))
		}
	}
	check := func(x bits) {
		t.Helper()
		v := f.decode(x)
		want := v.kind == finite && bid.IsCanonical128(x) && v.coeff.Cmp(pow(8)) >= 0
		if got := bid.IsLong128(x); got != want {
			t.Fatalf("IsLong128(%s) = %t, want %t", hex128(x), got, want)
		}
		if !want {
			return
		}
		ref := f.formatGo(x)
		var buf [bid.LongLen128]byte
		for i := range buf {
			buf[i] = '#'
		}
		n := bid.PutLong128(&buf, x)
		if n < 0 || n > len(buf) || string(buf[:n]) != ref {
			t.Fatalf("PutLong128(%s) = %q (n = %d), want %q", hex128(x), buf[:max(0, min(n, len(buf)))], n, ref)
		}
		for i := n; i < len(buf); i++ {
			if buf[i] != '#' {
				t.Fatalf("PutLong128(%s) wrote byte %d, after its %d-byte result %q", hex128(x), i, n, ref)
			}
		}
		// Append128 with a prefix, at each amount of spare capacity around
		// what the long path asks for, and the bytes after the result in use.
		for _, room := range []int{0, len(ref), bid.LongLen128 - 1, bid.LongLen128, bid.LongLen128 + 9} {
			b := make([]byte, 2+room+1)
			for i := range b {
				b[i] = '#'
			}
			out := bid.Append128(b[:2:2+room], x)
			if string(out) != "##"+ref {
				t.Fatalf("Append128(%s) with room %d = %q, want %q", hex128(x), room, out, "##"+ref)
			}
			if &out[0] == &b[0] {
				for i := len(out); i < len(b); i++ {
					if b[i] != '#' {
						t.Fatalf("Append128(%s) with room %d wrote byte %d, after its %d-byte result", hex128(x), room, i, len(out))
					}
				}
			}
		}
	}
	count := 0
	for _, c := range coeffs {
		n := len(c.String())
		var exps []int
		for e := -n - 8; e <= 3; e++ { // the point from eight places left of the digits to past their end
			exps = append(exps, e)
		}
		// The thresholds of exponential notation, by the leading digit's exponent.
		for _, adj := range []int{-5, -4, -3, 20, 21, 22, 99, 100, 101, 999, 1000, 1001, -99, -100, -101, -999, -1000} {
			exps = append(exps, adj-(n-1))
		}
		exps = append(exps, f.maxExp, f.minExp, f.maxExp-1, f.minExp+1)
		for range 4 {
			exps = append(exps, f.minExp+r.IntN(f.maxExp-f.minExp+1))
		}
		for _, exp := range exps {
			if exp < f.minExp || exp > f.maxExp {
				continue
			}
			for _, neg := range []bool{false, true} {
				x := f.encode(value{kind: finite, neg: neg, coeff: c, exp: exp})
				check(x)
				count++
			}
		}
	}
	// What IsLong128 must refuse or accept at its edges: a coefficient on
	// each side of 10^8 and of the largest, the second form and the
	// special values.
	for _, hi := range []uint64{0, 1 << 63, 0x3040 << 48, 0x3040<<48 | 1, 0x3040<<48 | 0x1ED09BEAD87C0, 0x3040<<48 | 0x1ED09BEAD87C1, 0x3040<<48 | 1<<48, 3 << 61, 3<<61 | 1<<60, 0x78 << 56, 0x7c << 56, 0x7e << 56, 0xfc << 56, ^uint64(0)} {
		for _, lo := range []uint64{0, 1, 99_999_999, 100_000_000, 100_000_001, 0x378D8E63FFFFFFFF, 0x378D8E6400000000, ^uint64(0)} {
			check(bits{Hi: hi, Lo: lo})
			count++
		}
	}
	t.Logf("%d encodings", count)
}

// TestEightQuads checks eightQuads against eightASCII, which TestEightASCII
// checks against the division loop, for every value below 10^8 (a stride of
// 7919 under -short), and the tables it and the exponent read against the
// digits of each index.
func TestEightQuads(t *testing.T) {
	for p, w := range bid.PairTab {
		want := uint16(0)
		if p < 100 {
			want = uint16('0'+p/10) | uint16('0'+p%10)<<8
		}
		if w != want {
			t.Fatalf("pairTab[%d] = %#04x, want %#04x", p, w, want)
		}
	}
	for n, w := range bid.QuadTab {
		if want := uint32('0'+n/1000) | uint32('0'+n/100%10)<<8 | uint32('0'+n/10%10)<<16 | uint32('0'+n%10)<<24; w != want {
			t.Fatalf("quadTab[%d] = %#08x, want %#08x", n, w, want)
		}
	}
	step := uint64(1)
	if testing.Short() {
		step = 7919
	}
	for v := uint64(0); v < 100_000_000; v += step {
		if got, want := bid.EightQuads(v), bid.EightASCII(v); got != want {
			t.Fatalf("eightQuads(%d) = %#016x, want %#016x", v, got, want)
		}
	}
	for _, v := range []uint64{0, 9, 10, 99, 100, 9_999, 10_000, 19_999, 999_999, 1_000_000, 99_990_000, 99_999_999} {
		if got, want := bid.EightQuads(v), bid.EightASCII(v); got != want {
			t.Fatalf("eightQuads(%d) = %#016x, want %#016x", v, got, want)
		}
	}
}

// TestLong64 checks Append64 on coefficients of nine digits or more, which
// it formats as the decimal128 of the same value, against the reference: the
// bounds of every length and a random sample of each, with the point at
// every place and the exponent across the format's range, each sign, and at
// each amount of spare capacity around what the long path asks for.
func TestLong64(t *testing.T) {
	f := dec64
	r := rand.New(rand.NewPCG(*seedFlag, streamSeed("long64")))
	var coeffs []uint64
	for p := uint64(100_000_000); p < 10_000_000_000_000_000; p *= 10 {
		coeffs = append(coeffs, p, p+1, 10*p-1, 10*p-2, 5*p)
		for range 60 {
			coeffs = append(coeffs, p+r.Uint64N(9*p))
		}
	}
	coeffs = append(coeffs, 99_999_999, 1<<53-1, 1<<53, 1<<53+1, 9_007_199_254_740_993)
	count := 0
	for _, c := range coeffs {
		n := len(new(big.Int).SetUint64(c).String())
		var exps []int
		for e := -n - 8; e <= 3; e++ {
			exps = append(exps, e)
		}
		for _, adj := range []int{-5, -4, -3, 20, 21, 22, 99, 100, 101, -99, -100, -101} {
			exps = append(exps, adj-(n-1))
		}
		exps = append(exps, f.maxExp, f.minExp, f.maxExp-1, f.minExp+1, f.minExp+r.IntN(f.maxExp-f.minExp+1))
		for _, exp := range exps {
			if exp < f.minExp || exp > f.maxExp {
				continue
			}
			for _, neg := range []bool{false, true} {
				x := f.encode(value{kind: finite, neg: neg, coeff: new(big.Int).SetUint64(c), exp: exp}).Lo
				ref := f.formatGo(bits{Lo: x})
				for _, room := range []int{0, len(ref), bid.LongLen128 - 1, bid.LongLen128, bid.LongLen128 + 9} {
					b := make([]byte, 2+room+1)
					for i := range b {
						b[i] = '#'
					}
					out := bid.Append64(b[:2:2+room], x)
					if string(out) != "##"+ref {
						t.Fatalf("Append64(%#016x) with room %d = %q, want %q", x, room, out, "##"+ref)
					}
					if &out[0] == &b[0] {
						for i := len(out); i < len(b); i++ {
							if b[i] != '#' {
								t.Fatalf("Append64(%#016x) with room %d wrote byte %d, after its %d-byte result", x, room, i, len(out))
							}
						}
					}
				}
				count++
			}
		}
	}
	t.Logf("%d encodings", count)
}
