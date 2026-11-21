// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go div_pow10.go.

package d128

// x / 10^pow10 with the residue of the discarded digits. Tolerant entry point:
// pow10 in 0...13000, in any relation to digitLen(x).
func div_128_pow10_checked_q128res(x u128, pow10 int) quot128Residue {

	if pow10 == 0 {
		return quot128Residue{x.dw0, x.dw1, const_RESIDUE_EXACT}
	}
	digitLen := digitLen_int_128(x.dw0, x.dw1)
	if pow10 < digitLen {
		return div_128_pow10_q128res(x, pow10)
	}
	var res residue
	if pow10 > digitLen {
		if digitLen > 0 {
			res = const_RESIDUE_LT_HALF
		} else {
			res = const_RESIDUE_EXACT
		}
	} else {
		res = residueFromDecadeWithDigitLen_128(x.dw0, x.dw1, digitLen)
	}
	return quot128Residue{0, 0, res}
}

// x / 10^pow10 with the residue of the discarded digits. Requires 1 <= pow10 <
// digitLen(x).
func div_128_pow10_q128res(x u128, pow10 int) quot128Residue {

	// pow10 == 1 keeps the divide-by-10 magic (single-digit residue lookup); a
	// single reciprocal-multiply kernel covers 2..38 uniformly.
	if pow10 == 1 {
		return div1e1_q128res_128_magic(x.dw0, x.dw1)
	}
	return divPow10_q128res_128_recipmul(x.dw0, x.dw1, pow10)
}

func div_256_pow10_q128res(x u256, pow10 int) quot128Residue {
	if pow10 == 0 {
		if !isZero_bool_128(x.dw2, x.dw3) {
			impossible("div_256_pow10_q128res: pow10 == 0 quotient overflows U128")
		}
		return quot128Residue{x.dw0, x.dw1, const_RESIDUE_EXACT}
	}
	if isZero_bool_128(x.dw2, x.dw3) {
		return div_128_pow10_q128res(x.lo(), pow10)
	}
	// 3x3 fast path: when the stripped dividend A = x>>(pow10-1) fits 3 limbs
	// (A < 2^191) the reciprocal also fits 3 limbs, so the recip multiply is a
	// 192x192 comba -- 9 macs vs the 4x4's 16 (DivPow10RecipMul192WorkOrder.md).
	// aBits(A) = bitLen(x)-(pow10-1), so A < 2^191  <=>  bitLen(x) <= 190 + pow10.
	if bitLen_int_256(x.dw0, x.dw1, x.dw2, x.dw3) <= 190+pow10 {
		return divPow10_q128res_192_recipmul(x, pow10)
	}
	// A single reciprocal-multiply kernel (D47) covers the whole 256-bit-dividend
	// q128res range pow10 1..77, replacing the former barrett(<=13)/barrett-loop(>13)
	// split: it is flat in k where the barrett-loop grew with k, so it is a win
	// across the range (and retires the rrmp10-256 machinery). Mirrors the 128-bit
	// recipmul collapse (D45).
	return divPow10_q128res_256_recipmul(x, pow10)
}
