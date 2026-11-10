// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"testing"
)

// BenchmarkKernelParseDrawn times the parsing of a 38-digit string, which
// rounds at both widths, on one fixed string and on strings drawn without a
// pattern from a pool: an experiment's benchmark, the same file at both
// commits of an A/B.
func BenchmarkKernelParseDrawn(b *testing.B) {
	const pool = 4096
	step := func(r uint64) uint64 {
		r ^= r << 13
		r ^= r >> 7
		r ^= r << 17
		return r
	}
	r := uint64(88172645463325252)
	drawn := make([]string, pool)
	for i := range drawn {
		var buf [39]byte
		for j := range buf {
			r = step(r)
			buf[j] = '0' + byte(r>>32%10)
		}
		if buf[0] == '0' {
			buf[0] = '1'
		}
		buf[34] = '.'
		drawn[i] = string(buf[:])
	}
	fixed := []string{"1234567890123456789012345678901234.5678"}
	var sink64 uint64
	var sink128 bid.Uint128
	for _, c := range []struct {
		name string
		xs   []string
	}{{"fixed", fixed}, {"drawn", drawn}} {
		b.Run("64/"+c.name, func(b *testing.B) {
			m := uint64(len(c.xs) - 1)
			r := uint64(88172645463325252)
			for i := 0; i < b.N; i++ {
				r = step(r)
				sink64, _ = bid.Parse64(c.xs[r>>32&m], bid.ToNearestEven)
			}
		})
		b.Run("128/"+c.name, func(b *testing.B) {
			m := uint64(len(c.xs) - 1)
			r := uint64(88172645463325252)
			for i := 0; i < b.N; i++ {
				r = step(r)
				sink128, _ = bid.Parse128(c.xs[r>>32&m], bid.ToNearestEven)
			}
		})
	}
	_, _ = sink64, sink128
}
