// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"testing"
)

// TestShortForm128 makes ShortForm128's documented stores into a buffer
// of exactly the form's length and checks the bytes against PutShort128's,
// for every coefficient below 10^5 and others to 10^8, at every count of
// places from 0 to 4, of either sign.
func TestShortForm128(t *testing.T) {
	check := func(sign, frac uint, c uint64) {
		var want [bid.ShortLen128]byte
		n := bid.PutShort128(&want, sign, frac, c)
		head, tail, total := bid.ShortForm128(frac, c)
		if int(sign+total) != n {
			t.Fatalf("sign %d frac %d c %d: length %d, PutShort128 %d", sign, frac, c, sign+total, n)
		}
		got := make([]byte, n)
		var scratch [16]byte
		got[0] = '-'
		half := total / 2
		got[sign] = byte(head)
		got[sign+half] = byte(head >> (8 * half & 63))
		got[n-1] = byte(tail >> 56)
		q := got
		if total < 4 {
			q = scratch[:]
		}
		put := func(i uint, w uint32) { q[i], q[i+1], q[i+2], q[i+3] = byte(w), byte(w>>8), byte(w>>16), byte(w>>24) }
		t4 := (total - 4) & 7
		put(sign, uint32(head))
		put(sign+t4/2, uint32(head>>(8*(t4/2))))
		put(sign+t4, uint32(tail>>32))
		if string(got) != string(want[:n]) {
			t.Fatalf("sign %d frac %d c %d: %q, PutShort128 %q", sign, frac, c, got, want[:n])
		}
	}
	for sign := uint(0); sign < 2; sign++ {
		for frac := uint(0); frac <= 4; frac++ {
			for c := uint64(0); c < 100_000; c++ {
				check(sign, frac, c)
			}
			for c := uint64(100_000); c < 100_000_000; c += 99_991 {
				check(sign, frac, c)
			}
		}
	}
}
