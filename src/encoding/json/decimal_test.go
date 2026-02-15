// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package json

import (
	"bytes"
	"strconv"
	"testing"
)

type decimalStruct struct {
	D64  decimal64  `json:"d64"`
	D128 decimal128 `json:"d128"`
	Name string     `json:"name"`
}

func TestDecimalMarshal(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{
			name: "decimal64 normal",
			in:   struct{ V decimal64 }{V: 3.14},
			want: `{"V":3.14}`,
		},
		{
			name: "decimal64 zero",
			in:   struct{ V decimal64 }{V: 0},
			want: `{"V":0}`,
		},
		{
			name: "decimal64 negative",
			in:   struct{ V decimal64 }{V: -42.5},
			want: `{"V":-42.5}`,
		},
		{
			name: "decimal128 normal",
			in:   struct{ V decimal128 }{V: 3.14},
			want: `{"V":3.14}`,
		},
		{
			name: "decimal128 zero",
			in:   struct{ V decimal128 }{V: 0},
			want: `{"V":0}`,
		},
		{
			name: "decimal128 negative",
			in:   struct{ V decimal128 }{V: -42.5},
			want: `{"V":-42.5}`,
		},
		{
			name: "struct with both decimals",
			in:   decimalStruct{D64: 1.5, D128: 2.5, Name: "test"},
			want: `{"d64":1.5,"d128":2.5,"name":"test"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Marshal(tt.in)
			if err != nil {
				t.Fatalf("Marshal(%v) error: %v", tt.in, err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal(%v) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestDecimalMarshalNaN(t *testing.T) {
	d64, _ := strconv.ParseDecimal64("NaN")
	_, err := Marshal(struct{ V decimal64 }{V: d64})
	if err == nil {
		t.Error("Marshal(NaN decimal64) should return error")
	}
}

func TestDecimalUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want decimalStruct
	}{
		{
			name: "normal values",
			in:   `{"d64":3.14,"d128":2.718,"name":"test"}`,
			want: decimalStruct{D64: 3.14, D128: 2.718, Name: "test"},
		},
		{
			name: "zero values",
			in:   `{"d64":0,"d128":0,"name":"zero"}`,
			want: decimalStruct{D64: 0, D128: 0, Name: "zero"},
		},
		{
			name: "negative values",
			in:   `{"d64":-1.5,"d128":-2.5,"name":"neg"}`,
			want: decimalStruct{D64: -1.5, D128: -2.5, Name: "neg"},
		},
		{
			name: "integer values",
			in:   `{"d64":42,"d128":100,"name":"int"}`,
			want: decimalStruct{D64: 42, D128: 100, Name: "int"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got decimalStruct
			err := Unmarshal([]byte(tt.in), &got)
			if err != nil {
				t.Fatalf("Unmarshal(%s) error: %v", tt.in, err)
			}
			if strconv.FormatDecimal64(got.D64, 'f', -1) != strconv.FormatDecimal64(tt.want.D64, 'f', -1) {
				t.Errorf("D64 = %v, want %v", got.D64, tt.want.D64)
			}
			if strconv.FormatDecimal128(got.D128, 'f', -1) != strconv.FormatDecimal128(tt.want.D128, 'f', -1) {
				t.Errorf("D128 = %v, want %v", got.D128, tt.want.D128)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %v, want %v", got.Name, tt.want.Name)
			}
		})
	}
}

func TestDecimalRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   decimalStruct
	}{
		{
			name: "normal",
			in:   decimalStruct{D64: 3.14, D128: 2.718, Name: "roundtrip"},
		},
		{
			name: "zero",
			in:   decimalStruct{D64: 0, D128: 0, Name: "zero"},
		},
		{
			name: "negative",
			in:   decimalStruct{D64: -99.99, D128: -123.456, Name: "neg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := Marshal(tt.in)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			var got decimalStruct
			err = Unmarshal(data, &got)
			if err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if strconv.FormatDecimal64(got.D64, 'f', -1) != strconv.FormatDecimal64(tt.in.D64, 'f', -1) {
				t.Errorf("D64 round trip: got %v, want %v", got.D64, tt.in.D64)
			}
			if strconv.FormatDecimal128(got.D128, 'f', -1) != strconv.FormatDecimal128(tt.in.D128, 'f', -1) {
				t.Errorf("D128 round trip: got %v, want %v", got.D128, tt.in.D128)
			}
			if got.Name != tt.in.Name {
				t.Errorf("Name round trip: got %v, want %v", got.Name, tt.in.Name)
			}
		})
	}
}

func TestDecimalStream(t *testing.T) {
	// Test using Encoder/Decoder (stream API)
	original := decimalStruct{D64: 1.23, D128: 4.56, Name: "stream"}

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(original); err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	var got decimalStruct
	dec := NewDecoder(&buf)
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	if strconv.FormatDecimal64(got.D64, 'f', -1) != strconv.FormatDecimal64(original.D64, 'f', -1) {
		t.Errorf("D64 = %v, want %v", got.D64, original.D64)
	}
	if strconv.FormatDecimal128(got.D128, 'f', -1) != strconv.FormatDecimal128(original.D128, 'f', -1) {
		t.Errorf("D128 = %v, want %v", got.D128, original.D128)
	}
}
