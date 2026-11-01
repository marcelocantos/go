// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements functions for untyped constant operands.

package types2

import (
	"cmd/compile/internal/syntax"
	"go/constant"
	"go/token"
	"internal/bid"
	. "internal/types/errors"
	"math"
	"math/big"
)

// overflow checks that the constant x is representable by its type.
// For untyped constants, it checks that the value doesn't become
// arbitrarily large.
func (check *Checker) overflow(x *operand, opPos syntax.Pos) {
	assert(x.mode() == constant_)

	if x.val.Kind() == constant.Unknown {
		// TODO(gri) We should report exactly what went wrong. At the
		//           moment we don't have the (go/constant) API for that.
		//           See also TODO in go/constant/value.go.
		check.error(atPos(opPos), InvalidConstVal, "constant result is not representable")
		x.invalidate() // go.dev/issue/81628
		return
	}

	// Typed constants must be representable in
	// their type after each constant operation.
	// x.typ cannot be a type parameter (type
	// parameters cannot be constant types).
	if isTyped(x.typ()) {
		check.representable(x, x.typ().Underlying().(*Basic))
		return
	}

	// Untyped integer values must not grow arbitrarily.
	const prec = 512 // 512 is the constant precision
	if x.val.Kind() == constant.Int && constant.BitLen(x.val) > prec {
		op := opName(x.expr)
		if op != "" {
			op += " "
		}
		check.errorf(atPos(opPos), InvalidConstVal, "constant %soverflow", op)
		x.invalidate() // go.dev/issue/81628
		return
	}

	// String values must not become arbitrarily long (go.dev/issue/78346).
	const maxLen = int64(2e9) // cmd/internal/obj.MaxSymSize
	if x.val.Kind() == constant.String {
		len := constant.StringLen(x.val)
		if len > maxLen {
			check.errorf(atPos(opPos), InvalidConstVal, "constant string too long (%d bytes > %d bytes)", len, maxLen)
			x.invalidate() // go.dev/issue/81628
			return
		}
	}
}

// representableConst reports whether x can be represented as
// value of the given basic type and for the configuration
// provided (only needed for int/uint sizes).
//
// If rounded != nil, *rounded is set to the rounded value of x for
// representable floating-point and complex values, and to an Int
// value for integer values; it is left alone otherwise.
// It is ok to provide the addressof the first argument for rounded.
//
// The check parameter may be nil if representableConst is invoked
// (indirectly) through an exported API call (AssignableTo, ConvertibleTo)
// because we don't need the Checker's config for those calls.
func representableConst(x constant.Value, check *Checker, typ *Basic, rounded *constant.Value) bool {
	if !isConstType(typ) {
		return false
	}

	if x.Kind() == constant.Unknown {
		return true // avoid follow-up errors
	}

	var conf *Config
	if check != nil {
		conf = check.conf
	}

	sizeof := func(T Type) int64 {
		s := conf.sizeof(T)
		return s
	}

	switch {
	case isInteger(typ):
		x := constant.ToInt(x)
		if x.Kind() != constant.Int {
			return false
		}
		if rounded != nil {
			*rounded = x
		}
		if x, ok := constant.Int64Val(x); ok {
			switch typ.kind {
			case Int:
				var s = uint(sizeof(typ)) * 8
				return int64(-1)<<(s-1) <= x && x <= int64(1)<<(s-1)-1
			case Int8:
				const s = 8
				return -1<<(s-1) <= x && x <= 1<<(s-1)-1
			case Int16:
				const s = 16
				return -1<<(s-1) <= x && x <= 1<<(s-1)-1
			case Int32:
				const s = 32
				return -1<<(s-1) <= x && x <= 1<<(s-1)-1
			case Int64, UntypedInt:
				return true
			case Uint, Uintptr:
				if s := uint(sizeof(typ)) * 8; s < 64 {
					return 0 <= x && x <= int64(1)<<s-1
				}
				return 0 <= x
			case Uint8:
				const s = 8
				return 0 <= x && x <= 1<<s-1
			case Uint16:
				const s = 16
				return 0 <= x && x <= 1<<s-1
			case Uint32:
				const s = 32
				return 0 <= x && x <= 1<<s-1
			case Uint64:
				return 0 <= x
			default:
				panic("unreachable")
			}
		}
		// x does not fit into int64
		switch n := constant.BitLen(x); typ.kind {
		case Uint, Uintptr:
			var s = uint(sizeof(typ)) * 8
			return constant.Sign(x) >= 0 && n <= int(s)
		case Uint64:
			return constant.Sign(x) >= 0 && n <= 64
		case UntypedInt:
			return true
		}

	case isFloat(typ):
		x := constant.ToFloat(x)
		if x.Kind() != constant.Float {
			return false
		}
		switch typ.kind {
		case Float32:
			if rounded == nil {
				return fitsFloat32(x)
			}
			r := roundFloat32(x)
			if r != nil {
				*rounded = r
				return true
			}
		case Float64:
			if rounded == nil {
				return fitsFloat64(x)
			}
			r := roundFloat64(x)
			if r != nil {
				*rounded = r
				return true
			}
		case UntypedFloat:
			return true
		default:
			panic("unreachable")
		}

	case isComplex(typ):
		x := constant.ToComplex(x)
		if x.Kind() != constant.Complex {
			return false
		}
		switch typ.kind {
		case Complex64:
			if rounded == nil {
				return fitsFloat32(constant.Real(x)) && fitsFloat32(constant.Imag(x))
			}
			re := roundFloat32(constant.Real(x))
			im := roundFloat32(constant.Imag(x))
			if re != nil && im != nil {
				*rounded = constant.BinaryOp(re, token.ADD, constant.MakeImag(im))
				return true
			}
		case Complex128:
			if rounded == nil {
				return fitsFloat64(constant.Real(x)) && fitsFloat64(constant.Imag(x))
			}
			re := roundFloat64(constant.Real(x))
			im := roundFloat64(constant.Imag(x))
			if re != nil && im != nil {
				*rounded = constant.BinaryOp(re, token.ADD, constant.MakeImag(im))
				return true
			}
		case UntypedComplex:
			return true
		default:
			panic("unreachable")
		}

	case isDecimal(typ):
		x := constant.ToFloat(x)
		if x.Kind() != constant.Float {
			return false
		}
		var fits bool
		switch typ.kind {
		case Decimal64:
			fits = fitsDecimal(x, maxDecimal64)
		case Decimal128:
			fits = fitsDecimal(x, maxDecimal128)
		default:
			panic("unreachable")
		}
		// A typed constant has the value its type can hold, as for floats, so
		// that comparing it agrees with comparing a variable that holds it.
		if fits && rounded != nil {
			*rounded = roundDecimal(x, typ.kind)
		}
		return fits

	case isString(typ):
		return x.Kind() == constant.String

	case isBoolean(typ):
		return x.Kind() == constant.Bool
	}

	return false
}

func fitsFloat32(x constant.Value) bool {
	f32, _ := constant.Float32Val(x)
	f := float64(f32)
	return !math.IsInf(f, 0)
}

func roundFloat32(x constant.Value) constant.Value {
	f32, _ := constant.Float32Val(x)
	f := float64(f32)
	if !math.IsInf(f, 0) {
		return constant.MakeFloat64(f)
	}
	return nil
}

func fitsFloat64(x constant.Value) bool {
	f, _ := constant.Float64Val(x)
	return !math.IsInf(f, 0)
}

func roundFloat64(x constant.Value) constant.Value {
	f, _ := constant.Float64Val(x)
	if !math.IsInf(f, 0) {
		return constant.MakeFloat64(f)
	}
	return nil
}

// toBigFloat extracts a *big.Float from a constant.Value.
// The value must be of kind constant.Float (i.e., after constant.ToFloat).
func toBigFloat(x constant.Value) *big.Float {
	v := constant.Val(x)
	switch v := v.(type) {
	case *big.Float:
		return v
	case *big.Rat:
		return new(big.Float).SetPrec(256).SetRat(v)
	case int64:
		return new(big.Float).SetPrec(256).SetInt64(v)
	default:
		return nil
	}
}

// roundDecimal returns the value x, which fits the decimal type kind, as the
// kernel rounds it at run time: to nearest, ties to even. A value that needs no
// rounding is returned as it is. So is one beyond 2^±4096, which only decimal128
// can hold: go/constant holds it inexactly, so only the compiler, which has the
// literal, can round it correctly.
func roundDecimal(x constant.Value, kind BasicKind) constant.Value {
	r, ok := constant.Val(x).(*big.Rat)
	if !ok || r.Sign() == 0 {
		return x
	}
	abs := new(big.Rat).Abs(r)

	// Hand the kernel the integer part of abs × 10^-exp, with sticky standing
	// for the rest. An estimate within 1.3 of log10(abs) gives it between one
	// and three digits more than the format holds, which fit its coefficient.
	const log10Of2 = 0.30103
	estimate := int(float64(abs.Num().BitLen()-abs.Denom().BitLen()) * log10Of2)
	precision := bid.Precision64
	if kind == Decimal128 {
		precision = bid.Precision128
	}
	exp := estimate - precision - 1
	num, den := abs.Num(), abs.Denom()
	power := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(max(exp, -exp))), nil)
	if exp < 0 {
		num = new(big.Int).Mul(num, power)
	} else {
		den = new(big.Int).Mul(den, power)
	}
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	sticky := rem.Sign() != 0

	coeff := new(big.Int)
	var cexp int
	if kind == Decimal128 {
		lo := new(big.Int).And(q, new(big.Int).SetUint64(math.MaxUint64)).Uint64()
		hi := new(big.Int).Rsh(q, 64).Uint64()
		bits, _ := bid.Round128(false, bid.Uint128{Hi: hi, Lo: lo}, exp, sticky, bid.ToNearestEven)
		_, _, c, e := bid.Unpack128(bits)
		coeff.SetUint64(c.Hi)
		coeff.Lsh(coeff, 64)
		coeff.Or(coeff, new(big.Int).SetUint64(c.Lo))
		cexp = e
	} else {
		bits, _ := bid.Round64(false, q.Uint64(), exp, sticky, bid.ToNearestEven)
		_, _, c, e := bid.Unpack64(bits)
		coeff.SetUint64(c)
		cexp = e
	}

	power = new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(max(cexp, -cexp))), nil)
	result := new(big.Rat).SetInt(coeff)
	if cexp < 0 {
		result.Quo(result, new(big.Rat).SetInt(power))
	} else {
		result.Mul(result, new(big.Rat).SetInt(power))
	}
	if result.Cmp(abs) == 0 {
		return x
	}
	if r.Sign() < 0 {
		result.Neg(result)
	}
	return constant.ToFloat(constant.Make(result))
}

// maxDecimal64 is the maximum finite decimal64 value: 9.999999999999999e+384
var maxDecimal64 = func() *big.Float {
	f, _, _ := new(big.Float).SetPrec(256).Parse("9.999999999999999e+384", 10)
	return f
}()

// maxDecimal128 is the maximum finite decimal128 value: 9.999999999999999999999999999999999e+6144
var maxDecimal128 = func() *big.Float {
	f, _, _ := new(big.Float).SetPrec(256).Parse("9.999999999999999999999999999999999e+6144", 10)
	return f
}()

// fitsDecimal reports whether the magnitude of x is at most max.
func fitsDecimal(x constant.Value, max *big.Float) bool {
	bf := toBigFloat(x)
	if bf == nil {
		return false
	}
	if bf.Sign() == 0 {
		return true
	}
	abs := new(big.Float).SetPrec(256).Abs(bf)
	return abs.Cmp(max) <= 0
}

// representable checks that a constant operand is representable in the given
// basic type.
func (check *Checker) representable(x *operand, typ *Basic) {
	v, code := check.representation(x, typ)
	if code != 0 {
		check.invalidConversion(code, x, typ)
		x.invalidate()
		return
	}
	assert(v != nil)
	x.val = v
}

// representation returns the representation of the constant operand x as the
// basic type typ.
//
// If no such representation is possible, it returns a non-zero error code.
func (check *Checker) representation(x *operand, typ *Basic) (constant.Value, Code) {
	assert(x.mode() == constant_)
	v := x.val
	if !representableConst(x.val, check, typ, &v) {
		if isNumeric(x.typ()) && isNumeric(typ) {
			// numeric conversion : error msg
			//
			// integer -> integer : overflows
			// integer -> float   : overflows (actually not possible)
			// float   -> integer : truncated
			// float   -> float   : overflows
			//
			if !isInteger(x.typ()) && isInteger(typ) {
				return nil, TruncatedFloat
			} else {
				return nil, NumericOverflow
			}
		}
		return nil, InvalidConstVal
	}
	return v, 0
}

// TODO(gri) There is overlap with the code and error messages here
//           and in conversions.go. Should simplify.

func (check *Checker) invalidConversion(code Code, x *operand, target Type) {
	msg := "cannot convert %s to type %s"
	switch code {
	case TruncatedFloat:
		msg = "%s truncated to %s"
	case NumericOverflow:
		msg = "%s overflows %s"
	}
	check.errorf(x, code, msg, x, target)
}

// convertUntyped attempts to set the type of an untyped value to the target type.
func (check *Checker) convertUntyped(x *operand, target Type) {
	newType, val, code := check.implicitTypeAndValue(x, target)
	if code != 0 {
		t := target
		if !isTypeParam(target) {
			t = safeUnderlying(target)
		}
		check.invalidConversion(code, x, t)
		x.invalidate()
		return
	}
	if val != nil {
		x.val = val
		check.updateExprVal(x.expr, val)
	}
	if newType != x.typ() {
		x.typ_ = newType
		check.updateExprType(x.expr, newType, false)
	}
}
