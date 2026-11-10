// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package runtime_test

import (
	"math"
	"math/decimal"
	"strconv"
	"testing"
)

// decimal64: +0 and -0 must be the same map key.
func TestDecimal64NegativeZero(t *testing.T) {
	m := make(map[decimal64]bool, 0)
	posZero := decimal64(0)
	negZero := math.Decimal64frombits(1 << 63)

	m[posZero] = true
	m[negZero] = true // should overwrite
	if len(m) != 1 {
		t.Errorf("length = %d, want 1", len(m))
	}
	for k := range m {
		if math.Decimal64bits(k)>>63 == 0 {
			t.Error("wrong sign: expected -0 key")
		}
	}

	m = make(map[decimal64]bool, 0)
	m[negZero] = true
	m[posZero] = true // should overwrite
	if len(m) != 1 {
		t.Errorf("length = %d, want 1", len(m))
	}
	for k := range m {
		if math.Decimal64bits(k)>>63 != 0 {
			t.Error("wrong sign: expected +0 key")
		}
	}
}

// decimal64 NaN: each assignment creates a new entry.
func TestDecimal64MapNan(t *testing.T) {
	m := make(map[decimal64]int, 0)
	nan := decimal.NaN[decimal64]()

	m[nan] = 1
	m[nan] = 2
	m[nan] = 4
	if len(m) != 3 {
		t.Errorf("length = %d, want 3", len(m))
	}
	s := 0
	for k, v := range m {
		if k == k {
			t.Error("NaN key equals itself")
		}
		if v&(v-1) != 0 {
			t.Errorf("value %d not a power of 2", v)
		}
		s |= v
	}
	if s != 7 {
		t.Errorf("values OR = %d, want 7", s)
	}
}

// Non-canonical BID representations of the same value must be equal
// and map to the same key.
func TestDecimal64NonCanonical(t *testing.T) {
	// Three representations of 100:
	//   coeff=100, exp=0 (biasedExp=398=0x18E): 0x31C0000000000064
	//   coeff=10,  exp=1 (biasedExp=399=0x18F): 0x31E000000000000A
	//   coeff=1,   exp=2 (biasedExp=400=0x190): 0x3200000000000001
	a := math.Decimal64frombits(0x31C0000000000064)
	b := math.Decimal64frombits(0x31E000000000000A)
	c := math.Decimal64frombits(0x3200000000000001)

	if a != b || b != c || a != c {
		t.Errorf("non-canonical values not equal: a=%v b=%v c=%v", a, b, c)
	}

	m := make(map[decimal64]int)
	m[a] = 1
	m[b] = 2
	m[c] = 3
	if len(m) != 1 {
		t.Errorf("map length = %d, want 1 (non-canonical keys should coalesce)", len(m))
	}
	if m[a] != 3 {
		t.Errorf("m[a] = %d, want 3", m[a])
	}
}

// decimal128 tests mirror the decimal64 tests above.
func TestDecimal128NegativeZero(t *testing.T) {
	m := make(map[decimal128]bool, 0)
	posZero := decimal128(0)
	negZero, _ := strconv.ParseDecimal("-0", 128)

	m[posZero] = true
	m[negZero] = true // should overwrite
	if len(m) != 1 {
		t.Errorf("length = %d, want 1", len(m))
	}

	m = make(map[decimal128]bool, 0)
	m[negZero] = true
	m[posZero] = true // should overwrite
	if len(m) != 1 {
		t.Errorf("length = %d, want 1", len(m))
	}
}

func TestDecimal128MapNan(t *testing.T) {
	m := make(map[decimal128]int, 0)
	nan, _ := strconv.ParseDecimal("NaN", 128)

	m[nan] = 1
	m[nan] = 2
	m[nan] = 4
	if len(m) != 3 {
		t.Errorf("length = %d, want 3", len(m))
	}
	s := 0
	for k, v := range m {
		if k == k {
			t.Error("NaN key equals itself")
		}
		if v&(v-1) != 0 {
			t.Errorf("value %d not a power of 2", v)
		}
		s |= v
	}
	if s != 7 {
		t.Errorf("values OR = %d, want 7", s)
	}
}
