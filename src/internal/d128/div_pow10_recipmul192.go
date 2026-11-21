// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go div_pow10_recipmul192.go.

package d128

// Full 192x192 -> 384 product, M supplied as its three dwords m0/m1/m2.
// A comba over the 256 kernel's accumulator and mac/emit helpers, 9 macs across
// 6 columns:
//
//	r0: a0*m0
//	r1: a1*m0 + a0*m1
//	r2: a2*m0 + a1*m1 + a0*m2
//	r3: a2*m1 + a1*m2
//	r4: a2*m2
//	r5: final carry
func mul384_192x192(a0, a1, a2, m0, m1, m2 uint64) (pLo, pHi u256) {
	// The column accumulator is macCol512's 192-bit one; the product comes back as
	// (pLo, pHi), each with a zero top limb (P < 2^383).
	var acc0, acc1, acc2 uint64

	macCol512(a0, m0, &acc0, &acc1, &acc2)
	r0 := acc0
	acc0, acc1, acc2 = acc1, acc2, 0

	macCol512(a1, m0, &acc0, &acc1, &acc2)
	macCol512(a0, m1, &acc0, &acc1, &acc2)
	r1 := acc0
	acc0, acc1, acc2 = acc1, acc2, 0

	macCol512(a2, m0, &acc0, &acc1, &acc2)
	macCol512(a1, m1, &acc0, &acc1, &acc2)
	macCol512(a0, m2, &acc0, &acc1, &acc2)
	r2 := acc0
	acc0, acc1, acc2 = acc1, acc2, 0

	macCol512(a2, m1, &acc0, &acc1, &acc2)
	macCol512(a1, m2, &acc0, &acc1, &acc2)
	r3 := acc0
	acc0, acc1, acc2 = acc1, acc2, 0

	macCol512(a2, m2, &acc0, &acc1, &acc2)
	r4 := acc0
	acc0, acc1, acc2 = acc1, acc2, 0

	r5 := acc0

	pLo = u256{int64(r0), int64(r1), int64(r2), 0}
	pHi = u256{int64(r3), int64(r4), int64(r5), 0}
	return pLo, pHi
}

// Dword accessors over the flat RMP10_192_M table: the three dwords of M live at
// [k*3 + 0/1/2] in ascending significance (m3 is implicitly 0 by the N=191 gate).
func rmp10_192_dw0(k int) int64 { return const_RMP10_192_M[k*3] }

func rmp10_192_dw1(k int) int64 { return const_RMP10_192_M[k*3+1] }

func rmp10_192_dw2(k int) int64 { return const_RMP10_192_M[k*3+2] }

// True iff any of the low `n` bits (0 <= n < 256) of x are set.
func lowBitsNonZero_192(d0, d1, d2, d3 int64, n int) bool {
	if n <= 0 {
		return false
	}
	u0, u1, u2, u3 := uint64(d0), uint64(d1), uint64(d2), uint64(d3)
	if n < 64 {
		return u0&((uint64(1)<<n)-1) != 0
	}
	if n == 64 {
		return u0 != 0
	}
	if n < 128 {
		return u0 != 0 || u1&((uint64(1)<<(n-64))-1) != 0
	}
	if n == 128 {
		return u0|u1 != 0
	}
	if n < 192 {
		return u0|u1 != 0 || u2&((uint64(1)<<(n-128))-1) != 0
	}
	if n == 192 {
		return u0|u1|u2 != 0
	}
	return u0|u1|u2 != 0 || u3&((uint64(1)<<(n-192))-1) != 0
}

// 3x3 recipmul kernel for x / 10^pow10 when the stripped dividend A = x >>
// (pow10 - 1) fits 3 limbs (A < 2^191), pow10 1..77. A narrowed sibling of the
// 4x4 divPow10_q128res_256_recipmul: the reciprocal M also fits 3 limbs, so
// the reciprocal multiply is a 192*192->384 comba, 9 64*64 macs against the
// 4x4's 16 (PrimitiveLayer.md D52). Same 128-bit quotient and classified
// residue. Backs Core's div_256_pow10_q128res dispatch when bitLen(x) <= 190 +
// pow10.
func divPow10_q128res_192_recipmul(x u256, pow10 int) quot128Residue {
	demand("recipmul192: pow10 outside [1, 77]", pow10 > 0 && pow10 <= 77)
	k := pow10
	m0 := rmp10_192_dw0(k)
	m1 := rmp10_192_dw1(k)
	m2 := rmp10_192_dw2(k)
	s192 := const_RMP10_192_S192[k] // = S192 - 192, in 1..173
	sh := k - 1                     // 0..76

	// Strip k-1 low bits (the 2^(k-1) factor of 10^k); track them as sticky.
	a := shr_256_256(x.dw0, x.dw1, x.dw2, x.dw3, sh)
	stickyAlfa := lowBitsNonZero_192(x.dw0, x.dw1, x.dw2, x.dw3, sh)

	// P = A * M  (384-bit), M = m2:m1:m0.
	pLo, pHi := mul384_192x192(uint64(a.dw0), uint64(a.dw1), uint64(a.dw2),
		uint64(m0), uint64(m1), uint64(m2))

	// quotPlusRound = P >> S192 = pHi >> s192  (= 2q + roundBit).
	quotPlusRound := shr_256_256(pHi.dw0, pHi.dw1, pHi.dw2, pHi.dw3, s192)
	q := shr_256_256(quotPlusRound.dw0, quotPlusRound.dw1, quotPlusRound.dw2, quotPlusRound.dw3, 1)
	roundBit := (quotPlusRound.dw0 & 1) != 0

	// stickyBeta = (P mod 2^S192) >= M: low s192 bits of pHi nonzero
	// (fraction >= 2^192 > M), else compare the low 192 bits (pLo) against M.
	highFrac := lowBitsNonZero_192(pHi.dw0, pHi.dw1, pHi.dw2, pHi.dw3, s192)
	pLoGeM := cmp_int_256x256(pLo.dw0, pLo.dw1, pLo.dw2, pLo.dw3, m0, m1, m2, 0) >= 0
	sticky := stickyAlfa || highFrac || pLoGeM

	res := const_RESIDUE_EXACT
	if roundBit && sticky {
		res = const_RESIDUE_GT_HALF
	} else if roundBit {
		res = const_RESIDUE_HALF
	} else if sticky {
		res = const_RESIDUE_LT_HALF
	}
	return quot128Residue{q.dw0, q.dw1, res}
}
