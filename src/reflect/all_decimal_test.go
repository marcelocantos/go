// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package reflect_test

import (
	. "reflect"
	"testing"
)

// The decimal cases of the tests in all_test.go. They live in their own
// file because the external test package is not compiled as part of the
// standard library, so it can name decimal64 and decimal128 only while
// GOEXPERIMENT=decimal is on.

func init() {
	valueTests = append(valueTests, []pair{
		{new(decimal64), "256.25"},
		{new(decimal128), "512.125"},
	}...)

	decimalSetValues[Decimal64] = ValueOf(decimal64(256.25))
	decimalSetValues[Decimal128] = ValueOf(decimal128(512.125))

	decimalCanTestCases = []canTestCase{
		// decimal
		{decimal64(256.25), [...]bool{false, false, false, false, true}},
		{decimal128(512.125), [...]bool{false, false, false, false, true}},
	}

	isZeroDecimalTests = []isZeroTest{
		// Decimals are zero by value, whatever the exponent and sign of the zero.
		{decimal64(0), true},
		{-decimal64(0), true},
		{decimal64(0.00), true},
		{decimal64(1.5), false},
		{decimal128(0), true},
		{-decimal128(0), true},
		{decimal128(0.00), true},
		{decimal128(1.5), false},
	}

	convertTests = append(convertTests, []struct{ in, out Value }{
		// decimal <-> decimal
		{V(decimal64(1.5)), V(decimal64(1.5))},
		{V(decimal64(2.5)), V(decimal128(2.5))},
		{V(decimal128(3.5)), V(decimal64(3.5))},
		{V(decimal128(4.5)), V(decimal128(4.5))},
		// int -> decimal64
		{V(int(10)), V(decimal64(10))},
		{V(int8(11)), V(decimal64(11))},
		{V(int16(12)), V(decimal64(12))},
		{V(int32(13)), V(decimal64(13))},
		{V(int64(14)), V(decimal64(14))},
		// int -> decimal128
		{V(int(15)), V(decimal128(15))},
		{V(int8(16)), V(decimal128(16))},
		{V(int16(17)), V(decimal128(17))},
		{V(int32(18)), V(decimal128(18))},
		{V(int64(19)), V(decimal128(19))},
		// uint -> decimal64
		{V(uint(20)), V(decimal64(20))},
		{V(uint8(21)), V(decimal64(21))},
		{V(uint16(22)), V(decimal64(22))},
		{V(uint32(23)), V(decimal64(23))},
		{V(uint64(24)), V(decimal64(24))},
		{V(uintptr(25)), V(decimal64(25))},
		// uint -> decimal128
		{V(uint(26)), V(decimal128(26))},
		{V(uint8(27)), V(decimal128(27))},
		{V(uint16(28)), V(decimal128(28))},
		{V(uint32(29)), V(decimal128(29))},
		{V(uint64(30)), V(decimal128(30))},
		{V(uintptr(31)), V(decimal128(31))},
		// float -> decimal64 (decimal64 has limited precision, so round-trip is exact for small integers)
		{V(float32(32)), V(decimal64(32))},
		{V(float64(34)), V(decimal64(34))},
		// float -> decimal128: appended in init() to force runtime conversion for expected values
		// decimal -> float
		{V(decimal64(62)), V(float32(62))},
		{V(decimal64(63)), V(float64(63))},
		{V(decimal128(64)), V(float32(64))},
		{V(decimal128(65)), V(float64(65))},
		// decimal64 -> int/uint
		{V(decimal64(40)), V(int(40))},
		{V(decimal64(41)), V(int8(41))},
		{V(decimal64(42)), V(int16(42))},
		{V(decimal64(43)), V(int32(43))},
		{V(decimal64(44)), V(int64(44))},
		{V(decimal64(45)), V(uint(45))},
		{V(decimal64(46)), V(uint8(46))},
		{V(decimal64(47)), V(uint16(47))},
		{V(decimal64(48)), V(uint32(48))},
		{V(decimal64(49)), V(uint64(49))},
		{V(decimal64(50)), V(uintptr(50))},
		// decimal128 -> int/uint
		{V(decimal128(51)), V(int(51))},
		{V(decimal128(52)), V(int8(52))},
		{V(decimal128(53)), V(int16(53))},
		{V(decimal128(54)), V(int32(54))},
		{V(decimal128(55)), V(int64(55))},
		{V(decimal128(56)), V(uint(56))},
		{V(decimal128(57)), V(uint8(57))},
		{V(decimal128(58)), V(uint16(58))},
		{V(decimal128(59)), V(uint32(59))},
		{V(decimal128(60)), V(uint64(60))},
		{V(decimal128(61)), V(uintptr(61))},
		// decimal -> float
		{V(decimal64(62)), V(float32(62))},
		{V(decimal64(63)), V(float64(63))},
		{V(decimal128(64)), V(float32(64))},
		{V(decimal128(65)), V(float64(65))},
		// truncation: decimal -> int
		{V(decimal64(1.9)), V(int(1))},
		{V(decimal128(2.9)), V(int(2))},
	}...)

	// float -> decimal128 entries are added at runtime so that the expected
	// decimal128 values are produced by the same float64->decimal128 conversion
	// used at runtime, rather than by the compiler's constant folding which
	// produces a different BID128 encoding for the same mathematical value.
	var f32 float32 = 33
	var f64 float64 = 35
	convertTests = append(convertTests,
		struct{ in, out Value }{V(f32), V(decimal128(float64(f32)))},
		struct{ in, out Value }{V(f64), V(decimal128(f64))},
	)

}

func TestDecimalReflect(t *testing.T) {
	// decimal64 round-trip through Decimal/SetDecimal
	var d64 decimal64 = 3.14
	v64 := ValueOf(&d64).Elem()
	if v64.Kind() != Decimal64 {
		t.Errorf("Kind = %v, want Decimal64", v64.Kind())
	}
	got64 := v64.Decimal()
	if got64 != decimal128(d64) {
		t.Errorf("Decimal() = %v, want %v", got64, decimal128(d64))
	}
	v64.SetDecimal(99.5)
	if d64 != 99.5 {
		t.Errorf("after SetDecimal(99.5), d64 = %v", d64)
	}

	// decimal128 round-trip through Decimal/SetDecimal
	var d128 decimal128 = 123456.789
	v128 := ValueOf(&d128).Elem()
	if v128.Kind() != Decimal128 {
		t.Errorf("Kind = %v, want Decimal128", v128.Kind())
	}
	got128 := v128.Decimal()
	if got128 != d128 {
		t.Errorf("Decimal() = %v, want %v", got128, d128)
	}
	v128.SetDecimal(42.0)
	if d128 != 42.0 {
		t.Errorf("after SetDecimal(42.0), d128 = %v", d128)
	}

	// Cross-type: set decimal64 from decimal128 value
	v64.SetDecimal(decimal128(77.25))
	if d64 != 77.25 {
		t.Errorf("after SetDecimal(decimal128(77.25)), d64 = %v", d64)
	}
}

func TestDecimalPanics(t *testing.T) {
	// Decimal() on non-decimal should panic
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Decimal() on float64 did not panic")
			}
		}()
		ValueOf(float64(1.0)).Decimal()
	}()

	// SetDecimal() on non-decimal should panic
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("SetDecimal() on float64 did not panic")
			}
		}()
		var f float64 = 1.0
		ValueOf(&f).Elem().SetDecimal(1.0)
	}()

	// SetDecimal() on non-addressable should panic
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("SetDecimal() on non-addressable did not panic")
			}
		}()
		ValueOf(decimal64(1.0)).SetDecimal(2.0)
	}()
}

func TestDecimalConvert(t *testing.T) {
	// int -> decimal64
	v := ValueOf(int64(42)).Convert(TypeOf(decimal64(0)))
	if v.Kind() != Decimal64 {
		t.Errorf("int64->decimal64 Kind = %v, want Decimal64", v.Kind())
	}
	if got := v.Decimal(); got != 42 {
		t.Errorf("int64(42)->decimal64 = %v, want 42", got)
	}

	// int -> decimal128
	v = ValueOf(int64(100)).Convert(TypeOf(decimal128(0)))
	if v.Kind() != Decimal128 {
		t.Errorf("int64->decimal128 Kind = %v, want Decimal128", v.Kind())
	}
	if got := v.Decimal(); got != 100 {
		t.Errorf("int64(100)->decimal128 = %v, want 100", got)
	}

	// uint -> decimal64
	v = ValueOf(uint64(7)).Convert(TypeOf(decimal64(0)))
	if got := v.Decimal(); got != 7 {
		t.Errorf("uint64(7)->decimal64 = %v, want 7", got)
	}

	// decimal64 -> decimal128
	v = ValueOf(decimal64(3.14)).Convert(TypeOf(decimal128(0)))
	if v.Kind() != Decimal128 {
		t.Errorf("decimal64->decimal128 Kind = %v, want Decimal128", v.Kind())
	}
	if got := v.Decimal(); got != decimal128(decimal64(3.14)) {
		t.Errorf("decimal64(3.14)->decimal128 = %v, want %v", got, decimal128(decimal64(3.14)))
	}

	// decimal128 -> decimal64
	v = ValueOf(decimal128(2.5)).Convert(TypeOf(decimal64(0)))
	if v.Kind() != Decimal64 {
		t.Errorf("decimal128->decimal64 Kind = %v, want Decimal64", v.Kind())
	}

	// decimal -> int (truncation)
	v = ValueOf(decimal64(9.99)).Convert(TypeOf(int(0)))
	if got := v.Int(); got != 9 {
		t.Errorf("decimal64(9.99)->int = %v, want 9", got)
	}

	// decimal -> uint
	v = ValueOf(decimal128(15)).Convert(TypeOf(uint32(0)))
	if got := v.Uint(); got != 15 {
		t.Errorf("decimal128(15)->uint32 = %v, want 15", got)
	}

	// decimal -> float64
	v = ValueOf(decimal64(2.5)).Convert(TypeOf(float64(0)))
	if got := v.Float(); got != 2.5 {
		t.Errorf("decimal64(2.5)->float64 = %v, want 2.5", got)
	}

	// negative decimal -> int
	v = ValueOf(decimal64(-5)).Convert(TypeOf(int(0)))
	if got := v.Int(); got != -5 {
		t.Errorf("decimal64(-5)->int = %v, want -5", got)
	}

	// zero decimal -> int
	v = ValueOf(decimal128(0)).Convert(TypeOf(int(0)))
	if got := v.Int(); got != 0 {
		t.Errorf("decimal128(0)->int = %v, want 0", got)
	}

	// CanConvert checks
	d64Type := TypeOf(decimal64(0))
	d128Type := TypeOf(decimal128(0))
	intType := TypeOf(int(0))
	f64Type := TypeOf(float64(0))

	if !intType.ConvertibleTo(d64Type) {
		t.Error("int should be ConvertibleTo decimal64")
	}
	if !intType.ConvertibleTo(d128Type) {
		t.Error("int should be ConvertibleTo decimal128")
	}
	if !d64Type.ConvertibleTo(d128Type) {
		t.Error("decimal64 should be ConvertibleTo decimal128")
	}
	if !d128Type.ConvertibleTo(d64Type) {
		t.Error("decimal128 should be ConvertibleTo decimal64")
	}
	if !d64Type.ConvertibleTo(intType) {
		t.Error("decimal64 should be ConvertibleTo int")
	}
	if !d64Type.ConvertibleTo(f64Type) {
		t.Error("decimal64 should be ConvertibleTo float64")
	}
	if !f64Type.ConvertibleTo(d128Type) {
		t.Error("float64 should be ConvertibleTo decimal128")
	}
}
