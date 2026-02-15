// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gob

import (
	"bytes"
	"reflect"
	"strconv"
	"testing"
	"unsafe"
)

type decimalStruct struct {
	D64  decimal64
	D128 decimal128
	Name string
}

func TestDecimalEncodeDecode(t *testing.T) {
	tests := []struct {
		name string
		in   decimalStruct
	}{
		{
			name: "normal values",
			in:   decimalStruct{D64: 3.14, D128: 2.718, Name: "normal"},
		},
		{
			name: "zero values",
			in:   decimalStruct{D64: 0, D128: 0, Name: "zero"},
		},
		{
			name: "negative values",
			in:   decimalStruct{D64: -42.5, D128: -123.456, Name: "neg"},
		},
		{
			name: "integer values",
			in:   decimalStruct{D64: 100, D128: 200, Name: "int"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := NewEncoder(&buf)
			if err := enc.Encode(tt.in); err != nil {
				t.Fatalf("Encode error: %v", err)
			}

			var got decimalStruct
			dec := NewDecoder(&buf)
			if err := dec.Decode(&got); err != nil {
				t.Fatalf("Decode error: %v", err)
			}

			// Compare using bits since decimal comparison may differ for special values.
			d64In := *(*uint64)(unsafe.Pointer(&tt.in.D64))
			d64Got := *(*uint64)(unsafe.Pointer(&got.D64))
			if d64In != d64Got {
				t.Errorf("D64: got %v (bits %#x), want %v (bits %#x)",
					got.D64, d64Got, tt.in.D64, d64In)
			}

			d128In := *(*[2]uint64)(unsafe.Pointer(&tt.in.D128))
			d128Got := *(*[2]uint64)(unsafe.Pointer(&got.D128))
			if d128In != d128Got {
				t.Errorf("D128: got %v (bits %v), want %v (bits %v)",
					got.D128, d128Got, tt.in.D128, d128In)
			}

			if got.Name != tt.in.Name {
				t.Errorf("Name: got %q, want %q", got.Name, tt.in.Name)
			}
		})
	}
}

func TestDecimalBasicRoundTrip(t *testing.T) {
	// Test encoding and decoding bare decimal values (not in a struct).
	values := []any{
		decimal64(1.5),
		decimal64(0),
		decimal64(-7.25),
		decimal128(1.5),
		decimal128(0),
		decimal128(-7.25),
	}

	for _, v := range values {
		t.Run(reflect.TypeOf(v).String(), func(t *testing.T) {
			var buf bytes.Buffer
			enc := NewEncoder(&buf)
			if err := enc.Encode(v); err != nil {
				t.Fatalf("Encode(%v) error: %v", v, err)
			}

			result := reflect.New(reflect.TypeOf(v))
			dec := NewDecoder(&buf)
			if err := dec.Decode(result.Interface()); err != nil {
				t.Fatalf("Decode error: %v", err)
			}

			got := result.Elem().Interface()
			switch g := got.(type) {
			case decimal64:
				want := v.(decimal64)
				if strconv.FormatDecimal64(g, 'f', -1) != strconv.FormatDecimal64(want, 'f', -1) {
					t.Errorf("got %v, want %v", g, want)
				}
			case decimal128:
				want := v.(decimal128)
				if strconv.FormatDecimal128(g, 'f', -1) != strconv.FormatDecimal128(want, 'f', -1) {
					t.Errorf("got %v, want %v", g, want)
				}
			}
		})
	}
}

func TestDecimalNaN(t *testing.T) {
	// NaN values should encode and decode as the same bit pattern.
	d64NaN, _ := strconv.ParseDecimal64("NaN")
	d128NaN, _ := strconv.ParseDecimal128("NaN")

	t.Run("decimal64_NaN", func(t *testing.T) {
		var buf bytes.Buffer
		enc := NewEncoder(&buf)
		if err := enc.Encode(d64NaN); err != nil {
			t.Fatalf("Encode error: %v", err)
		}

		var got decimal64
		dec := NewDecoder(&buf)
		if err := dec.Decode(&got); err != nil {
			t.Fatalf("Decode error: %v", err)
		}

		inBits := *(*uint64)(unsafe.Pointer(&d64NaN))
		gotBits := *(*uint64)(unsafe.Pointer(&got))
		if inBits != gotBits {
			t.Errorf("NaN decimal64 bits: got %#x, want %#x", gotBits, inBits)
		}
	})

	t.Run("decimal128_NaN", func(t *testing.T) {
		var buf bytes.Buffer
		enc := NewEncoder(&buf)
		if err := enc.Encode(d128NaN); err != nil {
			t.Fatalf("Encode error: %v", err)
		}

		var got decimal128
		dec := NewDecoder(&buf)
		if err := dec.Decode(&got); err != nil {
			t.Fatalf("Decode error: %v", err)
		}

		inBits := *(*[2]uint64)(unsafe.Pointer(&d128NaN))
		gotBits := *(*[2]uint64)(unsafe.Pointer(&got))
		if inBits != gotBits {
			t.Errorf("NaN decimal128 bits: got %v, want %v", gotBits, inBits)
		}
	})
}

func TestDecimalInf(t *testing.T) {
	d64Inf, _ := strconv.ParseDecimal64("Inf")
	d128Inf, _ := strconv.ParseDecimal128("Inf")

	t.Run("decimal64_Inf", func(t *testing.T) {
		var buf bytes.Buffer
		enc := NewEncoder(&buf)
		if err := enc.Encode(d64Inf); err != nil {
			t.Fatalf("Encode error: %v", err)
		}

		var got decimal64
		dec := NewDecoder(&buf)
		if err := dec.Decode(&got); err != nil {
			t.Fatalf("Decode error: %v", err)
		}

		inBits := *(*uint64)(unsafe.Pointer(&d64Inf))
		gotBits := *(*uint64)(unsafe.Pointer(&got))
		if inBits != gotBits {
			t.Errorf("Inf decimal64 bits: got %#x, want %#x", gotBits, inBits)
		}
	})

	t.Run("decimal128_Inf", func(t *testing.T) {
		var buf bytes.Buffer
		enc := NewEncoder(&buf)
		if err := enc.Encode(d128Inf); err != nil {
			t.Fatalf("Encode error: %v", err)
		}

		var got decimal128
		dec := NewDecoder(&buf)
		if err := dec.Decode(&got); err != nil {
			t.Fatalf("Decode error: %v", err)
		}

		inBits := *(*[2]uint64)(unsafe.Pointer(&d128Inf))
		gotBits := *(*[2]uint64)(unsafe.Pointer(&got))
		if inBits != gotBits {
			t.Errorf("Inf decimal128 bits: got %v, want %v", gotBits, inBits)
		}
	})
}
