// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package reflect

const (
	// Decimal64 is the kind of a decimal64 value.
	Decimal64 Kind = decimal64Kind
	// Decimal128 is the kind of a decimal128 value.
	Decimal128 Kind = decimal128Kind
)

// CanDecimal reports whether Value.Decimal can be used without panicking.
func (v Value) CanDecimal() bool { return canDecimal(v) }

// Decimal returns v's underlying value as decimal128. It panics unless v
// has kind Decimal64 or Decimal128.
func (v Value) Decimal() decimal128 { return decimalValue(v) }

// SetDecimal sets v to x. It panics unless v is settable and has kind
// Decimal64 or Decimal128.
func (v Value) SetDecimal(x decimal128) { setDecimalValue(v, x) }
