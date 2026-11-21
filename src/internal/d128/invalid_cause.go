// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go invalid_cause.go.

package d128

// invalidCause tags an invalid-operation signal (diagnostic; not load-bearing).
// The full set lands with the arithmetic operations in Phase 3; parse needs only
// causeParseMalformed.
type invalidCause int

const (
	causeParseMalformed invalidCause = iota
	causeNanOperand
	causeSnanOperand
	causeMagnitudeSubtractionOfInfinities
	causeMulZeroByInfinity
	causeDivZeroByZero
	causeDivInfByInf
	causeDivByZeroInRemainderOp
	causeInfNumeratorInRemainderOp
	causeDivIntOverflowsCoefficient
	causeConvertNonFiniteToInteger
	causeSquareRootOfNegFiniteNonZero
	causeSquareRootOfNegInfinity
	causeQuantizeExactlyOneOperandIsInfinite
	causeQuantizeResultWouldExceedPrecision
	causeQuantumOfNonFinite
	causeLogOfNegNumber
	causePowerOfNegBaseNonIntegerExponent
	causeRootnBadArgs
	causeCompoundXLtNegOne
	causeOther
)
