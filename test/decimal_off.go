// run
//go:build !goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test that, while GOEXPERIMENT=decimal is off, a program that uses no
// decimals sees no new reflect kinds: the two kind values the experiment
// adds print as unknown kinds, however they are printed, and the kinds and
// values of the existing types print as they always have.

package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

func check(what, got, want string) {
	if got != want {
		panic(fmt.Sprintf("%s = %q, want %q", what, got, want))
	}
}

func main() {
	// The kinds a program without decimals can name end at UnsafePointer.
	if reflect.UnsafePointer != 26 {
		panic(fmt.Sprintf("UnsafePointer = %d, want 26", reflect.UnsafePointer))
	}
	for k := reflect.Kind(27); k <= 29; k++ {
		want := fmt.Sprintf("kind%d", int(k))
		check(fmt.Sprintf("Kind(%d).String()", int(k)), k.String(), want)
		check(fmt.Sprintf("Sprint(Kind(%d))", int(k)), fmt.Sprint(k), want)
		check(fmt.Sprintf("Sprintf(%%v, Kind(%d))", int(k)), fmt.Sprintf("%v", k), want)
		check(fmt.Sprintf("Sprintf(%%s, Kind(%d))", int(k)), fmt.Sprintf("%s", k), want)
		check(fmt.Sprintf("Sprintf(%%d, Kind(%d))", int(k)), fmt.Sprintf("%d", k), fmt.Sprint(int(k)))
		check(fmt.Sprintf("Sprint(ValueOf(Kind(%d)))", int(k)), fmt.Sprint(reflect.ValueOf(k)), want)
	}

	// Every value a program without decimals can make has an old kind, which
	// prints by its old name, and the value prints as before.
	for _, tt := range []struct {
		v          any
		kind, text string
	}{
		{false, "bool", "false"},
		{int(-1), "int", "-1"},
		{int8(-8), "int8", "-8"},
		{int16(-16), "int16", "-16"},
		{int32(-32), "int32", "-32"},
		{int64(-64), "int64", "-64"},
		{uint(1), "uint", "1"},
		{uint8(8), "uint8", "8"},
		{uint16(16), "uint16", "16"},
		{uint32(32), "uint32", "32"},
		{uint64(64), "uint64", "64"},
		{uintptr(0), "uintptr", "0"},
		{float32(1.5), "float32", "1.5"},
		{float64(2.5), "float64", "2.5"},
		{complex64(1 + 2i), "complex64", "(1+2i)"},
		{complex128(3 + 4i), "complex128", "(3+4i)"},
		{[1]int{7}, "array", "[7]"},
		{(chan int)(nil), "chan", "<nil>"},
		{(func())(nil), "func", "<nil>"},
		{map[string]int{"a": 1}, "map", "map[a:1]"},
		{(*int)(nil), "ptr", "<nil>"},
		{[]int{8}, "slice", "[8]"},
		{"s", "string", "s"},
		{struct{ X float64 }{0.1}, "struct", "{0.1}"},
		{unsafe.Pointer(nil), "unsafe.Pointer", "<nil>"},
	} {
		v := reflect.ValueOf(tt.v)
		k := v.Kind()
		if k > reflect.UnsafePointer {
			panic(fmt.Sprintf("Kind of %T = %d, beyond UnsafePointer", tt.v, int(k)))
		}
		check(fmt.Sprintf("ValueOf(%T).Kind().String()", tt.v), k.String(), tt.kind)
		check(fmt.Sprintf("Sprint(ValueOf(%T))", tt.v), fmt.Sprint(v), tt.text)
		check(fmt.Sprintf("Sprintf(%%v, ValueOf(%T))", tt.v), fmt.Sprintf("%v", v), tt.text)
	}
	var e any
	check("Sprint(ValueOf(nil))", fmt.Sprint(reflect.ValueOf(e)), "<invalid reflect.Value>")
	check("ValueOf(nil).Kind().String()", reflect.ValueOf(e).Kind().String(), "invalid")
}
