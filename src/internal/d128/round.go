// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go round.go.

package d128

// round is a C-style int-backed selector: the constants are true compile-time
// constants (immutable, foldable immediates), and the named type keeps the
// selector out of plain-int positions. The values are load-bearing: they index
// the packed bit-tables below (§6.4), so do not reorder.
type round int

const (
	const_ROUND_TIES_TO_EVEN    round = 0
	const_ROUND_TIES_TO_AWAY    round = 1
	const_ROUND_TOWARD_ZERO     round = 2
	const_ROUND_TOWARD_POSITIVE round = 3
	const_ROUND_TOWARD_NEGATIVE round = 4
)

const const_ULP_BIAS_MAP int64 = 0b0_00000000_00001110_00000000_00001100_00001000

// Whether a truncated magnitude rounds up under this direction, given the
// result sign, the parity of the retained low digit, and the discarded
// residue.
func needsRoundUp(rounding round, sign, isOdd bool, res residue) bool {
	return ulpBias(forMagnitude(rounding, sign), isOdd, res) != 0
}

// Bias for the magnitude-mapped rounding direction: 1 if the truncated value
// rounds up, 0 otherwise.
func ulpBias(roundingCode round, isOdd bool, res residue) int64 {

	biasMapEvenOdd := const_ULP_BIAS_MAP | (boolToInt64(isOdd) << 2)
	bitIndex := (int(roundingCode) << 3) + res
	return (biasMapEvenOdd >> uint(bitIndex)) & 1
}

// forMagnitude returns the rounding-direction code to apply when operating on the
// magnitude of a signed value with this rounding direction.
//
// For sign-magnitude representations, arithmetic is performed on the
// unsigned magnitude with the sign reattached at the end. This requires
// mapping the original (signed-space) rounding direction to an equivalent
// direction in magnitude space:
//
// | Direction          | Positive sign | Negative sign     |
// |--------------------|---------------|-------------------|
// | `tiesToEven`       | unchanged     | unchanged         |
// | `tiesToAway`       | unchanged     | unchanged         |
// | `towardZero`       | unchanged     | unchanged         |
// | `towardPositive`   | unchanged     | `towardNegative`  |
// | `towardNegative`   | unchanged     | `towardPositive`  |
//
// Rounding toward +inf on a negative value moves toward zero in magnitude
// space -- equivalent to rounding toward -inf on the magnitude. The two
// directional rules swap; the three sign-symmetric rules are unchanged.
//
// Operates on raw codes (in and out): the result immediately indexes the
// ulp-bias table in `ulpBias`, so composing in `Int` space avoids an
// enum round-trip in the hot path.
//
// ## Encoding
//
// Branchless via a packed lookup. The map holds 4-bit nybbles indexed by
// `(sign ? 8 : 0) | code`:
//
//	Positive sign (indices 0..4):  identity map (low 32 bits)
//	Negative sign (indices 8..12): swap of 3 and 4 (high 32 bits)
//
// - Parameter sign: `true` if the operand is negative, `false` if positive.
func forMagnitude(roundingCode round, sign bool) round {
	magnitudeMap := int64(0x0003421000043210)
	signBits := 0
	if sign {
		signBits = 8
	}
	index := signBits | int(roundingCode)
	shift := index << 2
	result := int((magnitudeMap >> uint(shift)) & 0xF)
	return round(result)
}

// Whether overflow under this rounding direction produces infinity
// (vs. the largest representable finite value).
//
// | Rounding direction | Result    |
// |--------------------|-----------|
// | `tiesToEven`       | `true`    |
// | `tiesToAway`       | `true`    |
// | `towardZero`       | `false`   |
// | `towardPositive`   | `!sign`   |
// | `towardNegative`   | `sign`    |
//
// ## Encoding
//
// Branchless via a packed lookup. Each rounding mode is assigned a 4-bit
// nybble; within a nybble:
//
//	bit 0 = answer when sign = false
//	bit 1 = answer when sign = true
//	bits 2, 3 = unused (zero)
//
// Per-mode nybble values:
//
// | Mode (code)          | sign=false | sign=true | nybble |
// |----------------------|------------|-----------|--------|
// | `tiesToEven`     (0) | 1          | 1         | `0x3`  |
// | `tiesToAway`     (1) | 1          | 1         | `0x3`  |
// | `towardZero`     (2) | 0          | 0         | `0x0`  |
// | `towardPositive` (3) | 1          | 0         | `0x1`  |
// | `towardNegative` (4) | 0          | 1         | `0x2`  |
//
// Nybbles are packed with code 0 in the lowest nybble:
//
//	code:     4     3     2     1     0
//	nybble:  0x2   0x1   0x0   0x3   0x3
//	table = 0x21033
//
// Lookup is `(table >> (code * 4 + signBit)) & 1`.
func roundingOverflowsToInfinity(rounding round, sign bool) bool {
	table := 0x21033
	shift := (int(rounding) << 2) + signBitOf(sign)
	return ((table >> uint(shift)) & 1) != 0
}

// Whether underflow under this rounding direction collapses to zero
// (vs. the smallest representable subnormal magnitude).
//
// | Rounding direction | Result    |
// |--------------------|-----------|
// | `tiesToEven`       | `true`    |
// | `tiesToAway`       | `true`    |
// | `towardZero`       | `true`    |
// | `towardPositive`   | `sign`    |
// | `towardNegative`   | `!sign`   |
//
// ## Encoding
//
// Branchless via a packed lookup. Each rounding mode is assigned a 4-bit
// nybble; within a nybble:
//
//	bit 0 = answer when sign = false
//	bit 1 = answer when sign = true
//	bits 2, 3 = unused (zero)
//
// Per-mode nybble values:
//
// | Mode (code)          | sign=false | sign=true | nybble |
// |----------------------|------------|-----------|--------|
// | `tiesToEven`     (0) | 1          | 1         | `0x3`  |
// | `tiesToAway`     (1) | 1          | 1         | `0x3`  |
// | `towardZero`     (2) | 1          | 1         | `0x3`  |
// | `towardPositive` (3) | 0          | 1         | `0x2`  |
// | `towardNegative` (4) | 1          | 0         | `0x1`  |
//
// Nybbles are packed with code 0 in the lowest nybble:
//
//	code:     4     3     2     1     0
//	nybble:  0x1   0x2   0x3   0x3   0x3
//	table = 0x12333
//
// Lookup is `(table >> (code * 4 + signBit)) & 1`.
func roundingUnderflowsToZero(rounding round, sign bool) bool {
	table := 0x12333
	shift := (int(rounding) << 2) + signBitOf(sign)
	return ((table >> uint(shift)) & 1) != 0
}
