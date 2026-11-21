// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go exception754.go.

package d128

// exception754 is the five IEEE 754-2019 exceptions; the value is the bit
// position within an int64 flag word.
type exception754 = int

const (
	const_INVALID_OPERATION exception754 = 0
	const_DIVISION_BY_ZERO  exception754 = 1
	const_OVERFLOW          exception754 = 2
	const_UNDERFLOW         exception754 = 3
	const_INEXACT           exception754 = 4
)

func exc754Mask(e exception754) int64 { return int64(1) << uint(e) }
