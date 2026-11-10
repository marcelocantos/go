// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"strconv"
	"testing"
)

// TestEightASCII checks the lane arithmetic of eightASCII against the
// division loop for every value below 10^8 (a stride of 7919 under -short).
func TestEightASCII(t *testing.T) {
	step := uint64(1)
	if testing.Short() {
		step = 7919
	}
	var want [8]byte
	for v := uint64(0); v < 100_000_000; v += step {
		w := bid.EightASCII(v)
		x := v
		for j := 7; j >= 0; j-- {
			want[j] = byte('0' + x%10)
			x /= 10
		}
		for j := range want {
			if got := byte(w >> (8 * j)); got != want[j] {
				t.Fatalf("eightASCII(%d) byte %d = %q, want %q", v, j, got, want[j])
			}
		}
	}
	// The bounds the proof names: the largest value of each level.
	for _, v := range []uint64{0, 9, 10, 99, 100, 9_999, 10_000, 99_999_999} {
		w := bid.EightASCII(v)
		var got [8]byte
		for j := range got {
			got[j] = byte(w >> (8 * j))
		}
		if string(got[:]) != strconv.FormatUint(v+100_000_000, 10)[1:] {
			t.Fatalf("eightASCII(%d) = %q", v, got[:])
		}
	}
}

// TestWriteDigits compares writeWordDigits and writeDigits with strconv and
// math/big on the boundaries of every chunk and on random values, and
// checks that no byte before the digits is written.
func TestWriteDigits(t *testing.T) {
	const canary = 0xA5
	var vals []uint64
	p := uint64(1)
	for range 19 {
		vals = append(vals, p-1, p, p+1)
		p *= 10
	}
	vals = append(vals, p-1) // 10^19 - 1, the largest low word
	r := rand.New(rand.NewPCG(1, 2))
	for range 10_000 {
		vals = append(vals, r.Uint64()%p)
	}
	for _, v := range vals {
		var buf [40]byte
		for i := range buf {
			buf[i] = canary
		}
		n := bid.WriteWordDigits(buf[:], v)
		if got, want := string(buf[len(buf)-n:]), strconv.FormatUint(v, 10); got != want {
			t.Fatalf("writeWordDigits(%d) = %q, want %q", v, got, want)
		}
		if buf[len(buf)-n-1] != canary {
			t.Fatalf("writeWordDigits(%d) wrote before its digits", v)
		}
	}
	his := []uint64{1, 9, 10, 99, 123, 999, 1000, 99_999_999, 100_000_000, 999_999_999_999_999} // up to 15 digits: c < 10^34
	ten19 := new(big.Int).Exp(big.NewInt(10), big.NewInt(19), nil)
	for _, hi := range his {
		for _, lo := range []uint64{0, 1, 9, 99_999_999, 100_000_000, 9_999_999_999_999_999, 10_000_000_000_000_000, p - 1, r.Uint64() % p} {
			var buf [40]byte
			for i := range buf {
				buf[i] = canary
			}
			n := bid.WriteDigits(buf[:], hi, lo)
			want := new(big.Int).Mul(new(big.Int).SetUint64(hi), ten19)
			want.Add(want, new(big.Int).SetUint64(lo))
			if got := string(buf[len(buf)-n:]); got != want.String() {
				t.Fatalf("writeDigits(%d, %d) = %q, want %q", hi, lo, got, want)
			}
			if buf[len(buf)-n-1] != canary {
				t.Fatalf("writeDigits(%d, %d) wrote before its digits", hi, lo)
			}
		}
	}
}

var digitSink int

func BenchmarkWriteWordDigits(b *testing.B) {
	for _, c := range []struct {
		name string
		v    uint64
	}{{"6", 123456}, {"8", 12345678}, {"16", 1234567890123456}, {"19", 1234567890123456789}} {
		b.Run(c.name, func(b *testing.B) {
			var buf [40]byte
			n := 0
			vs := [4]uint64{c.v, c.v + 1, c.v + 2, c.v + 3}
			for i := 0; i < b.N; i++ {
				n += bid.WriteWordDigits(buf[:], vs[i&3])
			}
			digitSink = n
		})
	}
}

func BenchmarkWriteDigits34(b *testing.B) {
	var buf [40]byte
	n := 0
	his := [4]uint64{123456789012345, 123456789012346, 123456789012347, 123456789012348}
	for i := 0; i < b.N; i++ {
		n += bid.WriteDigits(buf[:], his[i&3], 6789012345678901234)
	}
	digitSink = n
}
