// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package gob

import (
	"bytes"
	"reflect"
	"strconv"
	"strings"
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
				if strconv.FormatDecimal(decimal128(g), 'f', -1, 64) != strconv.FormatDecimal(decimal128(want), 'f', -1, 64) {
					t.Errorf("got %v, want %v", g, want)
				}
			case decimal128:
				want := v.(decimal128)
				if strconv.FormatDecimal(g, 'f', -1, 128) != strconv.FormatDecimal(want, 'f', -1, 128) {
					t.Errorf("got %v, want %v", g, want)
				}
			}
		})
	}
}

func TestDecimalNaN(t *testing.T) {
	// NaN values should encode and decode as the same bit pattern.
	d64NaN, _ := parseDecimal64("NaN")
	d128NaN, _ := strconv.ParseDecimal("NaN", 128)

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
	d64Inf, _ := parseDecimal64("Inf")
	d128Inf, _ := strconv.ParseDecimal("Inf", 128)

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

// A decimal of either width is sent as the decimal128 encoding of its value:
// the high word byte-reversed, as a float's bits are, then the low word. The
// bytes are written by hand: 1 is 0x3040000000000000 0x1 in decimal128,
// whose reversed high word 0x4030 is gob's two-byte uint fe 40 30.
func TestDecimalWire(t *testing.T) {
	want := []byte{0xfe, 0x40, 0x30, 0x01}
	for _, v := range []any{struct{ D decimal128 }{1}, struct{ D decimal64 }{1}} {
		var buf bytes.Buffer
		if err := NewEncoder(&buf).Encode(v); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(buf.Bytes(), want) {
			t.Errorf("%T: stream % x does not contain % x", v, buf.Bytes(), want)
		}
	}
}

// As a float of either width decodes into either float type, a decimal of
// either width decodes into either decimal type. Into decimal64 the value
// rounds, and a finite value beyond the largest decimal64 is an error, as
// one beyond the largest float32 is.
func TestDecimalAcrossWidths(t *testing.T) {
	type d64 struct{ D decimal64 }
	type d128 struct{ D decimal128 }
	text := func(d decimal128) string { return strconv.FormatDecimal(d, 'e', -1, 128) }
	for _, tt := range []struct {
		in, want string // want is the decimal64 result, "" for an error
	}{
		{"1.50", "1.50"},
		{"-0.00", "-0.00"},
		{"0e+5", "0e+5"},
		{"1.234567890123456789", "1.234567890123457"},
		{"9.999999999999999e384", "9.999999999999999e384"},
		{"1e385", ""},
		{"-1e6000", ""},
		{"Inf", "Inf"},
		{"-Inf", "-Inf"},
		{"1e-6000", "0e-398"},
	} {
		in, err := strconv.ParseDecimal(tt.in, 128)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := NewEncoder(&buf).Encode(d128{in}); err != nil {
			t.Fatal(err)
		}
		var got d64
		err = NewDecoder(&buf).Decode(&got)
		if tt.want == "" {
			if err == nil || !strings.Contains(err.Error(), "out of range") {
				t.Errorf("decimal128 %s into decimal64: %v, %v; want a range error", tt.in, got.D, err)
			}
			continue
		}
		want, _ := strconv.ParseDecimal(tt.want, 64)
		if err != nil || text(decimal128(got.D)) != text(want) {
			t.Errorf("decimal128 %s into decimal64 = %s, %v; want %s", tt.in, text(decimal128(got.D)), err, text(want))
		}

		// And back: a decimal64 widens exactly.
		buf.Reset()
		if err := NewEncoder(&buf).Encode(d64{decimal64(want)}); err != nil {
			t.Fatal(err)
		}
		var wide d128
		if err := NewDecoder(&buf).Decode(&wide); err != nil || text(wide.D) != text(want) {
			t.Errorf("decimal64 %s into decimal128 = %s, %v", tt.want, text(wide.D), err)
		}
	}
}

// A zero other than the zero value, 0.00 or -0, is sent and arrives with
// its quantum and sign; the destination's earlier value does not show
// through.
func TestDecimalZeroCohorts(t *testing.T) {
	type s struct{ D decimal64 }
	for _, in := range []string{"0.00", "-0", "0e+3"} {
		d, _ := strconv.ParseDecimal(in, 64)
		var buf bytes.Buffer
		if err := NewEncoder(&buf).Encode(s{decimal64(d)}); err != nil {
			t.Fatal(err)
		}
		got := s{9}
		if err := NewDecoder(&buf).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if g, w := strconv.FormatDecimal(decimal128(got.D), 'e', -1, 64), strconv.FormatDecimal(d, 'e', -1, 64); g != w {
			t.Errorf("%s arrived as %s", w, g)
		}
	}
}

func parseDecimal64(s string) (decimal64, error) {
	d, err := strconv.ParseDecimal(s, 64)
	return decimal64(d), err
}
