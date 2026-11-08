// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package decimalreflect provides decimal access for standard-library code.
// The corresponding public reflect and strconv declarations are experimental.
package decimalreflect

import (
	"internal/abi"
	"reflect"
	_ "strconv"
	_ "unsafe"
)

const (
	Decimal64  reflect.Kind = reflect.Kind(abi.Decimal64)
	Decimal128 reflect.Kind = reflect.Kind(abi.Decimal128)
)

//go:linkname Decimal reflect.decimalValue
func Decimal(v reflect.Value) decimal128

//go:linkname SetDecimal reflect.setDecimalValue
func SetDecimal(v reflect.Value, x decimal128)

//go:linkname FormatDecimal strconv.formatDecimal
func FormatDecimal(d decimal128, fmt byte, prec, bitSize int) string

//go:linkname ParseDecimal strconv.parseDecimal
func ParseDecimal(s string, bitSize int) (decimal128, error)

//go:linkname AppendDecimal strconv.appendDecimal
func AppendDecimal(dst []byte, d decimal128, fmt byte, prec, bitSize int) []byte

func CanDecimal(v reflect.Value) bool { return v.Kind() == Decimal64 || v.Kind() == Decimal128 }
