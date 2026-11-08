// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import "testing"

var operatorSink128 decimal128

// These benchmarks use source operators, so compiler paths and their runtime
// fallbacks are included. Rotating inputs keep the arithmetic inside the loop;
// the chain separately measures a loop-carried dependency.
func BenchmarkOperatorAdd128(b *testing.B) {
	type pair struct{ x, y decimal128 }
	hits := [4]pair{{123.45, 0.01}, {-12.00, -0.25}, {0.00, 19.25}, {18446744073709551615, 1}}
	misses := [4]pair{{123.45, 0.0001}, {-12.00, 0.25}, {1e+20, 0.01}, {1.234567890123456789012345678901234, 0.01}}
	for _, c := range []struct {
		name string
		args [4]pair
	}{
		{"hit", hits},
		{"miss", misses},
		{"mixed", [4]pair{hits[0], misses[0], hits[1], misses[1]}},
		{"nearby", [4]pair{{123.45, 0.0675}, {-12.00, 0.001}, {1.0, -1.00}, {0.001, -12.00}}},
		{"general", [4]pair{{1e20, 0.01}, {1.234567890123456789012345678901234, 0.01}, {-1e20, 0.001}, {1.234567890123456789012345678901234, -0.01}}},
	} {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				p := c.args[i%len(c.args)]
				operatorSink128 = p.x + p.y
			}
		})
	}
	b.Run("chain", func(b *testing.B) {
		x, y := hits[0].x, hits[0].y
		for i := 0; i < b.N; i++ {
			x += y
		}
		operatorSink128 = x
	})
}

func BenchmarkOperatorSub128(b *testing.B) {
	type pair struct{ x, y decimal128 }
	for _, c := range []struct {
		name string
		args [4]pair
	}{
		{"sameQuantum", [4]pair{{123.45, 0.01}, {-12.00, -0.25}, {0.00, 19.25}, {18446744073709551615, 1}}},
		{"nearby", [4]pair{{123.45, 0.0675}, {-12.00, 0.001}, {1.0, 1.00}, {0.001, 12.00}}},
		{"general", [4]pair{{1e20, 0.01}, {1.234567890123456789012345678901234, 0.01}, {-1e20, 0.001}, {1.234567890123456789012345678901234, -0.01}}},
	} {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				p := c.args[i%len(c.args)]
				operatorSink128 = p.x - p.y
			}
		})
	}
}

func BenchmarkOperatorMul128(b *testing.B) {
	type pair struct{ x, y decimal128 }
	hits := [4]pair{{123.45, 0.0675}, {-12.00, 0.25}, {0.00, -19.25}, {99999999999999999, 100000000000000001}}
	misses := [4]pair{{1e-6176, 0.1}, {1e6111, 1e1}, {18446744073709551615, 18446744073709551615}, {1.234567890123456789012345678901234, 0.01}}
	for _, c := range []struct {
		name string
		args [4]pair
	}{
		{"hit", hits},
		{"miss", misses},
		{"mixed", [4]pair{hits[0], misses[0], hits[1], misses[1]}},
	} {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				p := c.args[i%len(c.args)]
				operatorSink128 = p.x * p.y
			}
		})
	}
}
