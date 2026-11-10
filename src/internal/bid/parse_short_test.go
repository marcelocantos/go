// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/rand/v2"
	"strings"
	"testing"
)

// TestParseShort checks the short-numeral path of Parse64 and Parse128
// against the general scanner (parseLong64, parseLong128): the same bits
// and status for every string of up to six characters over a small
// alphabet, for random longer ones over a wider alphabet, and at the
// boundaries of the form. It also checks that the path, not the scanner,
// decides every short numeral of digits and one point, by counting them.
func TestParseShort(t *testing.T) {
	alphabet := []byte("019.-+")
	var corpus []string
	var gen func(prefix []byte, n int)
	gen = func(prefix []byte, n int) {
		if n == 0 {
			corpus = append(corpus, string(prefix))
			return
		}
		for _, c := range alphabet {
			gen(append(prefix, c), n-1)
		}
	}
	for n := 1; n <= 6; n++ {
		gen(nil, n)
	}
	wide := []byte("0123456789.-+eExIn ")
	r := rand.New(rand.NewPCG(3, 4))
	for range 200_000 {
		n := 1 + r.IntN(10)
		b := make([]byte, n)
		for i := range b {
			b[i] = wide[r.IntN(len(wide))]
		}
		corpus = append(corpus, string(b))
	}
	corpus = append(corpus, "", ".", "-.", "+.", "5.", ".5", "-.5", "00000000", "99999999", "-99999999", "+99999999",
		"0.0000000", "12345678", "123456789", "1234567.8", "12345.678", "1E5", "1e5", "Infinity", "-Infinity", "nan", "0.", "-0", "-0.00",
		strings.Repeat("9", 8)+".", "."+strings.Repeat("9", 7), "1."+strings.Repeat("0", 6), "12..3", "1.2.3", "1 2", "0000000.", "-0000000.")
	short := 0
	for _, s := range corpus {
		for _, mode := range []bid.Mode{bid.ToNearestEven, bid.TowardZero, bid.TowardNegative} {
			x, st := bid.Parse128(s, mode)
			if y, st2 := bid.ParseLong128(s, mode); x != y || st != st2 {
				t.Fatalf("Parse128(%q, %v) = %#x, %v; the general scanner gives %#x, %v", s, mode, x, st, y, st2)
			}
			x64, st := bid.Parse64(s, mode)
			if y, st2 := bid.ParseLong64(s, mode); x64 != y || st != st2 {
				t.Fatalf("Parse64(%q, %v) = %#x, %v; the general scanner gives %#x, %v", s, mode, x64, st, y, st2)
			}
		}
		body := strings.TrimLeft(s, "+-")
		if len(s)-len(body) <= 1 && len(body) >= 1 && len(body) <= 8 && body != "." &&
			strings.Count(body, ".") <= 1 && strings.Trim(body, "0123456789.") == "" {
			short++
		}
	}
	if short < 1000 {
		t.Fatalf("corpus too thin: %d short numerals", short)
	}
	t.Logf("%d strings, %d short numerals", len(corpus), short)
}
