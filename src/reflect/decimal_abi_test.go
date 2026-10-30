// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.regabiargs

package reflect_test

import (
	"reflect"
	"testing"
)

// TestDecimalCallABI exercises the compiler's integer-register convention for
// decimal values, including the second half of decimal128 and stack fallback.
func TestDecimalCallABI(t *testing.T) {
	type pair struct {
		A decimal64
		B decimal128
	}
	cases := []struct {
		name string
		fn   any
		args []any
		want []any
	}{
		{"scalar64", func(a decimal64) decimal64 { return a }, []any{decimal64(123.25)}, []any{decimal64(123.25)}},
		{"scalar128", func(a decimal128) decimal128 { return a }, []any{decimal128(1234567890123456789012345678.125)}, []any{decimal128(1234567890123456789012345678.125)}},
		{"mixed", func(a decimal64, b decimal128, c uint64) (uint64, decimal128, decimal64) { return c, b, a }, []any{decimal64(-12.5), decimal128(1234567890123456789012345678.125), uint64(0xabcdef)}, []any{uint64(0xabcdef), decimal128(1234567890123456789012345678.125), decimal64(-12.5)}},
		{"struct", func(a pair) pair { return a }, []any{pair{decimal64(14.25), decimal128(1234567890123456789012345678.125)}}, []any{pair{decimal64(14.25), decimal128(1234567890123456789012345678.125)}}},
		{"stack", func(a, b, c, d, e, f, g, h uint64, x decimal128) decimal128 { return x }, []any{uint64(1), uint64(2), uint64(3), uint64(4), uint64(5), uint64(6), uint64(7), uint64(8), decimal128(1234567890123456789012345678.125)}, []any{decimal128(1234567890123456789012345678.125)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := make([]reflect.Value, len(tc.args))
			for i, arg := range tc.args {
				in[i] = reflect.ValueOf(arg)
			}
			out := reflect.ValueOf(tc.fn).Call(in)
			if len(out) != len(tc.want) {
				t.Fatalf("got %d results, want %d", len(out), len(tc.want))
			}
			for i, v := range out {
				if got := v.Interface(); got != tc.want[i] {
					t.Errorf("result %d: got %v, want %v", i, got, tc.want[i])
				}
			}
		})
	}
}
