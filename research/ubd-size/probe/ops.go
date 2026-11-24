package main

import "math/decimal"

//go:noinline
func Add(x, y decimal128) decimal128 { return x + y }

//go:noinline
func Sub(x, y decimal128) decimal128 { return x - y }

//go:noinline
func Mul(x, y decimal128) decimal128 { return x * y }

//go:noinline
func Div(x, y decimal128) decimal128 { return x / y }

//go:noinline
func Neg(x decimal128) decimal128 { return -x }

//go:noinline
func Eq(x, y decimal128) bool { return x == y }

//go:noinline
func Lt(x, y decimal128) bool { return x < y }

//go:noinline
func IsZero(x decimal128) bool { return x == 0 }

//go:noinline
func Pos(x decimal128) bool { return x > 0 }

//go:noinline
func Quantize(x, y decimal128) decimal128 { return decimal.Quantize(x, y) }

//go:noinline
func Round(x decimal128) decimal128 { return decimal.Round(x) }

//go:noinline
func Signbit(x decimal128) bool { return decimal.Signbit(x) }

//go:noinline
func IsNaN(x decimal128) bool { return decimal.IsNaN(x) }

//go:noinline
func Abs(x decimal128) decimal128 { return decimal.Abs(x) }

//go:noinline
func Const() decimal128 { return 19.99 }

//go:noinline
func Sum(xs []decimal128) decimal128 {
	var t decimal128
	for _, x := range xs {
		t += x
	}
	return t
}

//go:noinline
func Add64(x, y decimal64) decimal64 { return x + y }

func main() {
	xs := []decimal128{1.25, 2.50}
	println(Add(1, 2), Sub(1, 2), Mul(1, 2), Div(1, 2), Neg(1), Eq(1, 2), Lt(1, 2), IsZero(1), Pos(1),
		Quantize(1, 2), Round(1), Signbit(1), IsNaN(1), Abs(1), Const(), Sum(xs), Add64(1, 2))
}
