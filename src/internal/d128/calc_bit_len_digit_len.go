// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go calc_bit_len_digit_len.go.

package d128

import "math/bits"

func bitLen_int_128(xDw0, xDw1 int64) int {
	if xDw1 != 0 {
		return 128 - bits.LeadingZeros64(uint64(xDw1))
	}
	return 64 - bits.LeadingZeros64(uint64(xDw0))
}

func digitLen_int_128(xDw0, xDw1 int64) int {
	return digitLen_int_128_withBitLen(xDw0, xDw1, bitLen_int_128(xDw0, xDw1))
}

func digitLen_int_128_withBitLen(xDw0, xDw1 int64, bitLen int) int {
	loDigitCount := (bitLen * 1233) >> 12
	hiDigitCount := loDigitCount + 1
	if cmp_int_128x128(xDw0, xDw1, const_POW10[(loDigitCount<<1)&const_POW10_BCE], const_POW10[((loDigitCount<<1)|1)&const_POW10_BCE]) < 0 {
		return loDigitCount
	}
	return hiDigitCount
}

func bitLen_int_256(xDw0, xDw1, xDw2, xDw3 int64) int {
	if (xDw2 | xDw3) != 0 {
		if xDw3 != 0 {
			return 256 - bits.LeadingZeros64(uint64(xDw3))
		}
		return 192 - bits.LeadingZeros64(uint64(xDw2))
	}
	if xDw1 != 0 {
		return 128 - bits.LeadingZeros64(uint64(xDw1))
	}
	return 64 - bits.LeadingZeros64(uint64(xDw0))
}

func digitLen_int_256(xDw0, xDw1, xDw2, xDw3 int64) int {
	if (xDw2 | xDw3) != 0 {
		return digitLen_int_256_withBitLen(xDw0, xDw1, xDw2, xDw3, bitLen_int_256(xDw0, xDw1, xDw2, xDw3))
	}
	return digitLen_int_128_withBitLen(xDw0, xDw1, bitLen_int_128(xDw0, xDw1))
}

func digitLen_int_256_withBitLen(xDw0, xDw1, xDw2, xDw3 int64, bitLen int) int {
	if bitLen <= 128 {
		return digitLen_int_128_withBitLen(xDw0, xDw1, bitLen)
	}
	// bitLen > 128: needs the POW10_256 lookup. 10^39 has bit length 130.
	loDigitCount := max((bitLen*1233)>>12, 39)
	b := loDigitCount - 39
	base := (const_POW10_QUAD_BASE + (b << 2)) & const_POW10_BCE
	pow := u256{const_POW10[base], const_POW10[base+1], const_POW10[base+2], const_POW10[base+3]}
	x := u256{xDw0, xDw1, xDw2, xDw3}
	if x.less(pow) {
		return loDigitCount
	}
	return loDigitCount + 1
}

// less is the unsigned U256 ordering (Swift's U256.<): compare high half, then
// low half.
func (x u256) less(y u256) bool {
	ch := cmp_int_128x128(x.dw2, x.dw3, y.dw2, y.dw3)
	if ch != 0 {
		return ch < 0
	}
	return cmp_int_128x128(x.dw0, x.dw1, y.dw0, y.dw1) < 0
}
