// run
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test that the zero value of a decimal type is the result of converting the
// integer constant 0 to that type, however the variable came to be zeroed,
// and that the conversion between memory form and BID is right at the edges.

package main

import (
	"fmt"
	"math/decimal"
	"reflect"
	"runtime"
	"unsafe"
)

var (
	global64     decimal64
	global128    decimal128
	static64     decimal64  = 0
	static128    decimal128 = 0
	staticQ64    decimal64  = 0.00
	staticQ128   decimal128 = 0.00
	staticOne64  decimal64  = 1.50
	staticNeg64  decimal64  = -1.50
	staticArr               = [2]decimal64{0, 0.00}
	staticStruct            = struct {
		a decimal64
		b decimal128
	}{0.00, 1.50}
)

type pair struct {
	a decimal64
	b decimal128
}

func check64(name string, got decimal64, want uint64) {
	if b := decimal.BID64bits(got); b != want {
		panic(fmt.Sprintf("%s: bits %#x, want %#x", name, b, want))
	}
}

func check128(name string, got decimal128, wantHi, wantLo uint64) {
	if hi, lo := decimal.BID128bits(got); hi != wantHi || lo != wantLo {
		panic(fmt.Sprintf("%s: bits %#x %#x, want %#x %#x", name, hi, lo, wantHi, wantLo))
	}
}

//go:noinline
func zero64() (d decimal64) { return }

//go:noinline
func zero128() (d decimal128) { return }

//go:noinline
func add64(x, y decimal64) decimal64 { return x + y }

//go:noinline
func add128(x, y decimal128) decimal128 { return x + y }

func main() {
	zero64Bits := decimal.BID64bits(decimal64(0))
	zero128Hi, zero128Lo := decimal.BID128bits(decimal128(0))

	if zero64Bits != 0x31c0000000000000 {
		panic(fmt.Sprintf("decimal64(0) bits %#x", zero64Bits))
	}
	if zero128Hi != 0x3040000000000000 || zero128Lo != 0 {
		panic(fmt.Sprintf("decimal128(0) bits %#x %#x", zero128Hi, zero128Lo))
	}

	// Every way of getting a zero value.
	var local64 decimal64
	var local128 decimal128
	var arr [3]decimal64
	var st pair
	sl := make([]decimal128, 2)
	m := map[string]decimal64{}
	check64("var", local64, zero64Bits)
	check128("var", local128, zero128Hi, zero128Lo)
	check64("global", global64, zero64Bits)
	check128("global", global128, zero128Hi, zero128Lo)
	check64("array", arr[2], zero64Bits)
	check64("struct", st.a, zero64Bits)
	check128("struct", st.b, zero128Hi, zero128Lo)
	check128("make", sl[1], zero128Hi, zero128Lo)
	check64("new", *new(decimal64), zero64Bits)
	check64("map miss", m["x"], zero64Bits)
	check64("named result", zero64(), zero64Bits)
	check128("named result", zero128(), zero128Hi, zero128Lo)
	check64("reflect.Zero", reflect.Zero(reflect.TypeFor[decimal64]()).Interface().(decimal64), zero64Bits)
	check128("reflect.Zero", reflect.Zero(reflect.TypeFor[decimal128]()).Interface().(decimal128), zero128Hi, zero128Lo)
	check64("static 0", static64, zero64Bits)
	check128("static 0", static128, zero128Hi, zero128Lo)

	// The zero value is all bits clear in memory.
	if *(*uint64)(unsafe.Pointer(&local64)) != 0 {
		panic("the zero value of decimal64 is not all bits clear")
	}
	if *(*[2]uint64)(unsafe.Pointer(&local128)) != [2]uint64{} {
		panic("the zero value of decimal128 is not all bits clear")
	}

	// An accumulator takes the quantum of what is added to it.
	var total64 decimal64
	total64 += 19.99
	check64("accumulator", total64, decimal.BID64bits(19.99))
	var total128 decimal128
	total128 += 19.99
	wantHi, wantLo := decimal.BID128bits(19.99)
	check128("accumulator", total128, wantHi, wantLo)
	check64("zero value + x", add64(local64, 1.50), decimal.BID64bits(1.50))
	wantHi, wantLo = decimal.BID128bits(1.50)
	check128("zero value + x", add128(local128, 1.50), wantHi, wantLo)

	// Other zeros are not the zero value, and stay distinct.
	if decimal.BID64bits(staticQ64) == zero64Bits {
		panic("0.00 lost its quantum in static data")
	}
	if hi, _ := decimal.BID128bits(staticQ128); hi == zero128Hi {
		panic("0.00 lost its quantum in static data")
	}
	if decimal.BID64bits(staticArr[1]) == zero64Bits || decimal.BID64bits(staticStruct.a) == zero64Bits {
		panic("0.00 lost its quantum in a static composite")
	}
	if staticArr[1] != 0 || staticStruct.a != 0 {
		panic("0.00 is not equal to zero")
	}
	if staticOne64 != 1.5 || staticNeg64 != -1.5 || staticStruct.b != 1.5 {
		panic("static data has the wrong value")
	}

	// Negation flips the sign alone, including at the zero value.
	neg := -local64
	if !decimal.Signbit[decimal64](neg) || neg != 0 {
		panic("-0 of the zero value")
	}
	if decimal.BID64bits(neg) != zero64Bits|1<<63 {
		panic(fmt.Sprintf("-zero bits %#x", decimal.BID64bits(neg)))
	}
	negHi, _ := decimal.BID128bits(-local128)
	if negHi != zero128Hi|1<<63 {
		panic(fmt.Sprintf("-zero bits %#x", negHi))
	}

	// Round trips through the bits functions, at both widths. A canonical
	// encoding survives on every target. A non-canonical one survives where
	// the memory form is BID; where it is DPD (linux/ppc64le), a value
	// holds the value the encoding reads as, and the bits functions return
	// its canonical encoding (see decimal.BID64bits).
	dpd := runtime.GOARCH == "ppc64le"
	for _, c := range []struct {
		b, canon64, canonHi, canonLo uint64
	}{
		{0, 0, 0, 0},
		{1, 1, 1, 1},
		{zero64Bits, zero64Bits, zero64Bits, zero64Bits},
		{zero64Bits | 1<<63, zero64Bits | 1<<63, zero64Bits | 1<<63, zero64Bits | 1<<63},
		{0x7800000000000000, 0x7800000000000000, 0x7800000000000000, 0}, // decimal128: an infinity with trailing bits
		{0x7c00000000000000, 0x7c00000000000000, 0x7c00000000000000, 0x7c00000000000000},
		{^uint64(0), 0xfe00000000000000, 0xfe00000000000000, 0}, // sNaN with reserved bits and a payload too large
	} {
		want64, wantHi, wantLo := c.b, c.b, c.b
		if dpd {
			want64, wantHi, wantLo = c.canon64, c.canonHi, c.canonLo
		}
		if got := decimal.BID64bits(decimal.BID64frombits(c.b)); got != want64 {
			panic(fmt.Sprintf("decimal64 round trip %#x got %#x, want %#x", c.b, got, want64))
		}
		if hi, lo := decimal.BID128bits(decimal.BID128frombits(c.b, c.b)); hi != wantHi || lo != wantLo {
			panic(fmt.Sprintf("decimal128 round trip %#x got %#x %#x, want %#x %#x", c.b, hi, lo, wantHi, wantLo))
		}
	}
	// Every zero hashes and compares the same in a map.
	keys := map[decimal64]int{}
	keys[local64]++
	keys[0.00]++
	keys[-local64]++
	if len(keys) != 1 || keys[0] != 3 {
		panic(fmt.Sprint("zeros are not one map key: ", keys))
	}
	if !reflect.ValueOf(local64).IsZero() || !reflect.ValueOf(staticQ64).IsZero() {
		panic("IsZero")
	}
}
