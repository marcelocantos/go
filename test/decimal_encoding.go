// run
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Exercise the public stored-bits and interchange-codec APIs through a
// compiled user program on both software BID and hardware DPD targets.
package main

import (
	"encoding/binary"
	"math"
	"math/decimal"
	"runtime"
)

func main() {
	if math.Decimal64bits(0) != 0 {
		panic("zero stored bits")
	}
	if hi, lo := math.Decimal128bits(0); hi != 0 || lo != 0 {
		panic("zero stored words")
	}
	const pattern = uint64(0x7dffffffffffffff)
	if math.Decimal64bits(math.Decimal64frombits(pattern)) != pattern {
		panic("raw bits changed")
	}
	if hi, lo := math.Decimal128bits(math.Decimal128frombits(pattern, ^pattern)); hi != pattern || lo != ^pattern {
		panic("raw words changed")
	}
	want := decimal.BIDXOR
	if runtime.GOARCH == "ppc64le" {
		want = decimal.DPDXOR
	}
	if decimal.NativeRepresentation != want {
		panic("native representation")
	}
	x := decimal64(1.50)
	y := decimal128(1.50)
	if a := decimal.BID64frombits(decimal.BID64bits(x)); math.Decimal64bits(a) != math.Decimal64bits(x) {
		panic("BID64 quantum")
	}
	if a := decimal.DPD64frombits(decimal.DPD64bits(x)); math.Decimal64bits(a) != math.Decimal64bits(x) {
		panic("DPD64 quantum")
	}
	hi, lo := math.Decimal128bits(y)
	for _, a := range []decimal128{
		decimal.BID128frombits(decimal.BID128bits(y)),
		decimal.DPD128frombits(decimal.DPD128bits(y)),
	} {
		if ah, al := math.Decimal128bits(a); ah != hi || al != lo {
			panic("decimal128 codec quantum")
		}
	}
	// Serialization retains its explicitly chosen BID wire contract; raw
	// representation introspection is a separate operation.
	var buf [8]byte
	if _, err := binary.Encode(buf[:], binary.BigEndian, decimal64(0)); err != nil {
		panic(err)
	}
	if binary.BigEndian.Uint64(buf[:]) != decimal.BID64bits(0) {
		panic("binary wire encoding")
	}
}
