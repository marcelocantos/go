// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go div_pow10_recipmul256.go.

package d128

import "math/bits"

// macCol512 adds the 64×64→128 product a*b into the 192-bit column accumulator
// acc0:acc1:acc2 (low→high), rippling carry into acc2. The Go spelling (no
// closures, §6.x regime) of Swift's UInt128 mac step in mul512_256x256.
func macCol512(a, b uint64, acc0, acc1, acc2 *uint64) {
	hi, lo := bits.Mul64(a, b)
	var c uint64
	*acc0, c = bits.Add64(*acc0, lo, 0)
	*acc1, c = bits.Add64(*acc1, hi, c)
	*acc2 += c
}

// Full 256x256 -> 512 product, M supplied as its two 128-bit limbs. A comba
// whose column accumulator never loses the carry of up to four 64x64 products:
// columns 0..3 of a 4-limb M = A*Mlo, columns 4..7 = A*Mhi, summed in place.
func mul512_256x256(a0, a1, a2, a3, m0, m1, m2, m3 uint64) (pLo, pHi u256) {
	// acc0:acc1:acc2 is the running column sum; each emit takes the finished low
	// limb and shifts the accumulator down 64 bits. The product comes back as (pLo, pHi).
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

	macCol512(a3, m0, &acc0, &acc1, &acc2)
	macCol512(a2, m1, &acc0, &acc1, &acc2)
	macCol512(a1, m2, &acc0, &acc1, &acc2)
	macCol512(a0, m3, &acc0, &acc1, &acc2)
	r3 := acc0
	acc0, acc1, acc2 = acc1, acc2, 0

	macCol512(a3, m1, &acc0, &acc1, &acc2)
	macCol512(a2, m2, &acc0, &acc1, &acc2)
	macCol512(a1, m3, &acc0, &acc1, &acc2)
	r4 := acc0
	acc0, acc1, acc2 = acc1, acc2, 0

	macCol512(a3, m2, &acc0, &acc1, &acc2)
	macCol512(a2, m3, &acc0, &acc1, &acc2)
	r5 := acc0
	acc0, acc1, acc2 = acc1, acc2, 0

	macCol512(a3, m3, &acc0, &acc1, &acc2)
	r6 := acc0
	acc0, acc1, acc2 = acc1, acc2, 0

	r7 := acc0

	pLo = u256{int64(r0), int64(r1), int64(r2), int64(r3)}
	pHi = u256{int64(r4), int64(r5), int64(r6), int64(r7)}
	return pLo, pHi
}

// Dword accessors over the single interleaved RMP10_256_M table (D50); dw0..dw3
// are the four 64-bit dwords of M in ascending significance. Mirrors
// pow10_256_dw*.
func rmp10_256_dw0(k int) int64 { return const_RMP10_256_M[k<<2] }

func rmp10_256_dw1(k int) int64 { return const_RMP10_256_M[(k<<2)|1] }

func rmp10_256_dw2(k int) int64 { return const_RMP10_256_M[(k<<2)|2] }

func rmp10_256_dw3(k int) int64 { return const_RMP10_256_M[(k<<2)|3] }

// True iff any of the low `n` bits (0 <= n < 256) of x are set.
func lowBitsNonZero_256(d0, d1, d2, d3 int64, n int) bool {
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

// Single reciprocal-multiply kernel for x / 10^pow10 (256-bit dividend,
// coefficient above 128 bits), pow10 1..77, returning the 128-bit quotient and
// the classified residue. One 256*256->512 multiply plus a shift, uniform
// across the whole range (PrimitiveLayer.md D47). Backs Core's
// div_256_pow10_q128res dispatch when the high half is nonzero.
func divPow10_q128res_256_recipmul(x u256, pow10 int) quot128Residue {
	demand("recipmul256: pow10 outside [1, 77]", pow10 > 0 && pow10 <= 77)
	k := pow10
	m0 := rmp10_256_dw0(k)
	m1 := rmp10_256_dw1(k)
	m2 := rmp10_256_dw2(k)
	m3 := rmp10_256_dw3(k)
	s256 := const_RMP10_256_S256[k] // = S - 256
	sh := k - 1                     // 0..76

	// Strip k-1 low bits (the 2^(k-1) factor of 10^k); track them as sticky.
	a := shr_256_256(x.dw0, x.dw1, x.dw2, x.dw3, sh)
	stickyAlfa := lowBitsNonZero_256(x.dw0, x.dw1, x.dw2, x.dw3, sh)

	// P = A * M  (512-bit), M = m3:m2:m1:m0.
	pLo, pHi := mul512_256x256(
		uint64(a.dw0), uint64(a.dw1), uint64(a.dw2), uint64(a.dw3),
		uint64(m0), uint64(m1), uint64(m2), uint64(m3))

	// quotPlusRound = P >> S = pHi >> s256  (= 2q + roundBit, < 2^129).
	quotPlusRound := shr_256_256(pHi.dw0, pHi.dw1, pHi.dw2, pHi.dw3, s256)
	q := shr_256_256(quotPlusRound.dw0, quotPlusRound.dw1, quotPlusRound.dw2, quotPlusRound.dw3, 1)
	roundBit := (quotPlusRound.dw0 & 1) != 0

	// stickyBeta = (P mod 2^S) >= M, without a 512-bit compare:
	//   low s256 bits of pHi nonzero  ->  fraction >= 2^256 > M
	//   else compare the low 256 bits (pLo) against M (= m3:m2:m1:m0).
	highFrac := lowBitsNonZero_256(pHi.dw0, pHi.dw1, pHi.dw2, pHi.dw3, s256)
	pLoGeM := cmp_int_256x256(pLo.dw0, pLo.dw1, pLo.dw2, pLo.dw3, m0, m1, m2, m3) >= 0
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
