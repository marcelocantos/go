// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go d128.go.

package d128

type d128 struct {
	ubdLo64 int64
	ubdHi64 int64
}

// d128Pack builds a finite d128 from sign + quantum exponent + coefficient dwords
// (the init(sign,qExp,coeff) mechanic; construction is platform-local).
func d128Pack(sign bool, qExp int, coeffDw0, coeffDw1 int64) d128 {

	qExpAndSignBit := (int64(qExp) << 50) | (boolToInt64(sign) << 49)
	return d128{ubdLo64: coeffDw0, ubdHi64: qExpAndSignBit | coeffDw1}
}

// d128FromQExpSignBit builds a finite d128 from a pre-packed qExpAndSignBit and
// coefficient (the init(qExpAndSignBit, coeff) mechanic).
func d128FromQExpSignBit(qExpAndSignBit, coeffDw0, coeffDw1 int64) d128 {

	return d128{ubdLo64: coeffDw0, ubdHi64: (qExpAndSignBit << 49) | coeffDw1}
}

func d128_hi49(x d128) int64 { return x.ubdHi64 & ((1 << 49) - 1) }

func d128_signBit(x d128) int { return int(ushr_64_64(x.ubdHi64, 49)) & 1 }

func d128_signFlag(x d128) bool { return d128_signBit(x) != 0 }

// The encoded exponent (quantum) of a finite x.
func d128_qExp(x d128) int { return int(x.ubdHi64 >> 50) }

func d128_qExpAndSignBit(x d128) int64 { return ushr_64_64(x.ubdHi64, 49) }

// The coefficient as a U128.
func d128_u128(x d128) u128 { return fromLoHi_128_64x64(x.ubdLo64, d128_hi49(x)) }

func d128_digitLen(x d128) int { return digitLen_int_128(x.ubdLo64, d128_hi49(x)) }

// Whether `x` is infinite or NaN.
func d128_isNonFinite(x d128) bool { return ushr_64_64(x.ubdHi64, 60) == 7 }

func d128_isFIN(x d128) bool { return ushr_64_64(x.ubdHi64, 60) != 7 }

// Whether `x` is zero: a finite value whose coefficient is zero.
func d128_isZER(x d128) bool { return ((x.ubdHi64 << 15) | x.ubdLo64) == 0 }

func d128_isINF(x d128) bool { return ushr_64_64(x.ubdHi64, 59) == 14 }

func d128_isNAN(x d128) bool { return ushr_64_64(x.ubdHi64, 59) == 15 }

func d128_isSNAN(x d128) bool { return ushr_64_64(x.ubdHi64, 58) == 31 }

func bothFinite(x, y d128) bool { return d128_isFIN(x) && d128_isFIN(y) }

const (
	const_D128_NON_FINITE_BIAS int64 = 0x1_F000_0000_0000
	const_D128_INF_HI64        int64 = 14 << 59
	const_D128_NAN_HI64        int64 = 15 << 59
)

func d128_infinity(sign bool) d128 {
	signBit := boolToInt64(sign) << 49
	return d128{ubdLo64: 0, ubdHi64: const_D128_INF_HI64 | signBit | const_D128_NON_FINITE_BIAS}
}

var (
	const_D128_QNAN0        = d128{ubdLo64: 0, ubdHi64: const_D128_NAN_HI64 | const_D128_NON_FINITE_BIAS}
	const_D128_SNAN0        = d128{ubdLo64: 0, ubdHi64: const_D128_NAN_HI64 | (1 << 58) | const_D128_NON_FINITE_BIAS}
	const_D128_POS_INFINITY = d128_infinity(false)
	const_D128_NEG_INFINITY = d128_infinity(true)
)

func d128_quieted(x d128) d128 {

	return d128{ubdLo64: x.ubdLo64, ubdHi64: x.ubdHi64 &^ (int64(1) << 58)}
}

func d128_zero(sign bool, qExp int) d128 {
	qClamped := min(max(qExp, -6176), 6111)
	return d128Pack(sign, qClamped, 0, 0)
}

func decimalFNZ_128(sign bool, qExp int, coeffDw0, coeffDw1 int64) d128 {

	return d128Pack(sign, qExp, coeffDw0, coeffDw1)
}

// Limb-native FNZ: pack the D128 straight from the coefficient dwords, without
// building a U128.
func decimalFNZ_64x64(sign bool, qExp int, dw0, dw1 int64) d128 {

	qExpAndSignBit := (int64(qExp) << 50) | (boolToInt64(sign) << 49)
	return d128{ubdLo64: dw0, ubdHi64: qExpAndSignBit | dw1}
}
