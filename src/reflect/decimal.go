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

// OverflowDecimal reports whether the decimal128 x cannot be represented by
// v's type, as [Value.OverflowFloat] does for floats: only a finite x beyond
// the type's largest finite value counts, not one that rounds.
// It panics if v's Kind is not [Decimal64] or [Decimal128].
func (v Value) OverflowDecimal(x decimal128) bool {
	switch v.kind() {
	case Decimal64:
		return overflowDecimal64(x)
	case Decimal128:
		return false
	}
	panic(&ValueError{"reflect.Value.OverflowDecimal", v.kind()})
}

func overflowDecimal64(x decimal128) bool {
	const (
		maxDecimal64  decimal128 = 9.999999999999999e384
		maxDecimal128 decimal128 = 9.999999999999999999999999999999999e6144
	)
	if x < 0 {
		x = -x
	}
	return maxDecimal64 < x && x <= maxDecimal128
}
