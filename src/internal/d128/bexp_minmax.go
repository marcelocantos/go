// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go bexp_minmax.go.

package d128

// log2(10) ≈ const / 2^32 (FLOOR <= true <= CEIL).
const (
	const_LOG2_10_FLOOR = 14_267_572_564
	const_LOG2_10_CEIL  = 14_267_572_565
)

// calcBExpMin computes a conservative **lower bound** on the binary exponent of a finite
// decimal value whose coefficient has the given `bitLen` and whose decimal
// exponent is `qExp`.
//
// ## Binary-Exponent Interval
//
// A finite decimal value can be written as:
//
//	x = coeff * 10^qExp
//
// Its exact (base-2) magnitude satisfies:
//
//	log2(|x|) = log2(coeff) + qExp * log2(10)
//
// Let:
//
//	bExp = floor(log2(|x|))
//
// This function computes **bExpMin**, a lower bound such that:
//
//	bExpMin <= bExp
//
// The true exponent always lies in the interval:
//
//	[bExpMin, bExpMax]
//
// ## Integer Approximation of `qExp * log2(10)`
//
// We approximate `qExp * log2(10)` using fixed-point multiplication:
//
//	log2(10) ~= LOG2_10_{FLOOR,CEIL} / 2^32
//
// where LOG2_10_FLOOR and LOG2_10_CEIL are exact integer bounds such that:
//
//	LOG2_10_FLOOR / 2^32 <= log2(10) <= LOG2_10_CEIL / 2^32
//
// For `qExp >= 0` we multiply by `LOG2_10_FLOOR` (still <= true value).
// For `qExp <  0` we multiply by `LOG2_10_CEIL`  (more negative, still <= true value).
//
// The product is then shifted right by 32 bits, which performs:
//
//	floor(scaled / 2^32)
//
// for both positive and negative values, because arithmetic right-shift in
// two's-complement is mathematically equal to floor division by powers of two.
//
// ## Final Lower Bound
//
// The coefficient contributes `(bitLen - 1)` to the binary exponent range.
// Adding the scaled `qExp` term yields:
//
//	bExpMin = floor((bitLen - 1) + qExp * log2(10))
//
// The result is further masked so that a zero-length coefficient (`bitLen == 0`)
// yields `bExpMin = 0`.
//
// This function is branch-free and guaranteed correct: the returned value is
// always <= the true binary exponent, never above it.
//
//   - Note: This assumes a 64-bit `Int`. The non-zero and sign masks use
//     `&>> 63`, which matches the bit-width of `Int` on all current Apple
//     platforms and 64-bit Linux. `bitLen` and `qExp` are expected to fit
//     comfortably within 32 bits in practice; the 64-bit width simply gives
//     more headroom for the intermediate `qExp * LOG2_10` product.
func calcBExpMin(bitLen, qExp int) int {
	bitLenNonZeroMask := (0 - bitLen) >> 63
	bitLenLess1 := bitLen - 1
	scaledFloor := (qExp * const_LOG2_10_FLOOR) >> 32
	scaledCeil := (qExp * const_LOG2_10_CEIL) >> 32
	bExpMinWhenPos := (bitLenLess1 + scaledFloor) & bitLenNonZeroMask
	bExpMinWhenNeg := (bitLenLess1 + scaledCeil) & bitLenNonZeroMask
	signMask := qExp >> 63
	return (bExpMinWhenNeg & signMask) | (bExpMinWhenPos &^ signMask)
}

// calcBExpMax computes a conservative **upper bound** on the binary exponent of a finite
// decimal value whose coefficient has the given `bitLen` and whose decimal
// exponent is `qExp`.
//
// ## Binary-Exponent Upper Bound
//
// For:
//
//	x = coeff * 10^qExp
//
// let:
//
//	bExp = floor(log2(|x|))
//
// This function computes **bExpMax**, an upper bound such that:
//
//	bExp <= bExpMax
//
// Together with `bExpMin`, this forms the binary-exponent interval:
//
//	[bExpMin, bExpMax]
//
// ## Deriving the Upper Bound
//
// We want:
//
//	bExpMax >= ceil(bitLen + qExp * log2(10)) - 1
//
// To compute the ceiling of `qExp * log2(10)` using only integer arithmetic,
// we use the same fixed-point scaling as `calcBExpMin`, but invert signs so
// that a simple arithmetic right-shift produces a ceiling instead of a floor.
//
// For any value `x`:
//
//	ceil(x) = -floor(-x)
//
// Thus we multiply by the negated LOG2_10 constant:
//
//	q >= 0 -> use -LOG2_10_CEIL  (upper bound)
//	q <  0 -> use -LOG2_10_FLOOR
//
// Then shift right by 32 bits, which computes:
//
//	floor(negatedScaled / 2^32)
//
// and negating again yields the desired ceiling.
//
// Combining coefficient and exponent terms produces:
//
//	bExpMax = ceil(bitLen + qExp * log2(10)) - 1
//
// Finally, a mask forces the result to zero when `bitLen == 0`.
//
// This function is branch-free and guaranteed correct: the returned value is
// always >= the true binary exponent, never below it.
func calcBExpMax(bitLen, qExp int) int {
	bitLenNonZeroMask := (0 - bitLen) >> 63
	scaledNegCeil := (qExp * (0 - const_LOG2_10_CEIL)) >> 32
	scaledNegFloor := (qExp * (0 - const_LOG2_10_FLOOR)) >> 32
	bExpMaxWhenPos := (bitLen - scaledNegCeil - 1) & bitLenNonZeroMask
	bExpMaxWhenNeg := (bitLen - scaledNegFloor - 1) & bitLenNonZeroMask
	signMask := qExp >> 63
	return (bExpMaxWhenNeg & signMask) | (bExpMaxWhenPos &^ signMask)
}
