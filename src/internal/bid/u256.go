// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// Uint256 is a 256-bit unsigned integer, most significant half first. It is
// the exact product of two 128-bit coefficients and the dividend of the
// decimal128 division, and it is the wide type of the decimal128
// instantiation of the shared core (see "Width parameterisation" in the
// package documentation).
type Uint256 struct {
	Hi, Lo Uint128
}

// u256Add returns a + b modulo 2^256 and the carry out of bit 255 (0 or 1).
func u256Add(a, b Uint256) (sum Uint256, carry uint64) {
	sum.Lo.Lo, carry = bits.Add64(a.Lo.Lo, b.Lo.Lo, 0)
	sum.Lo.Hi, carry = bits.Add64(a.Lo.Hi, b.Lo.Hi, carry)
	sum.Hi.Lo, carry = bits.Add64(a.Hi.Lo, b.Hi.Lo, carry)
	sum.Hi.Hi, carry = bits.Add64(a.Hi.Hi, b.Hi.Hi, carry)
	return sum, carry
}

// u256Sub returns a - b modulo 2^256 and the borrow out of bit 255 (0 or 1;
// 1 exactly when a < b).
func u256Sub(a, b Uint256) (diff Uint256, borrow uint64) {
	diff.Lo.Lo, borrow = bits.Sub64(a.Lo.Lo, b.Lo.Lo, 0)
	diff.Lo.Hi, borrow = bits.Sub64(a.Lo.Hi, b.Lo.Hi, borrow)
	diff.Hi.Lo, borrow = bits.Sub64(a.Hi.Lo, b.Hi.Lo, borrow)
	diff.Hi.Hi, borrow = bits.Sub64(a.Hi.Hi, b.Hi.Hi, borrow)
	return diff, borrow
}

// u256Cmp returns -1, 0 or +1 as a is less than, equal to or greater than b.
func u256Cmp(a, b Uint256) int {
	if c := u128Cmp(a.Hi, b.Hi); c != 0 {
		return c
	}
	return u128Cmp(a.Lo, b.Lo)
}

// u256Lt reports whether a < b.
func u256Lt(a, b Uint256) bool {
	if a.Hi != b.Hi {
		return u128Lt(a.Hi, b.Hi)
	}
	return u128Lt(a.Lo, b.Lo)
}

// u256Len returns the length of a in bits: 0 for zero, else the position of
// the highest set bit plus one.
func u256Len(a Uint256) int {
	if a.Hi != (Uint128{}) {
		return 128 + u128Len(a.Hi)
	}
	return u128Len(a.Lo)
}

// u256Shl returns a << n. Bits shifted out are lost; a shift of 256 or more
// yields zero.
func u256Shl(a Uint256, n uint) Uint256 {
	if n >= 128 {
		return Uint256{Hi: u128Shl(a.Lo, n-128)}
	}
	return Uint256{
		Hi: u128Or(u128Shl(a.Hi, n), u128Shr(a.Lo, 128-n)),
		Lo: u128Shl(a.Lo, n),
	}
}

// u256Shr returns a >> n (a logical shift). A shift of 256 or more yields
// zero.
func u256Shr(a Uint256, n uint) Uint256 {
	if n >= 128 {
		return Uint256{Lo: u128Shr(a.Hi, n-128)}
	}
	return Uint256{
		Hi: u128Shr(a.Hi, n),
		Lo: u128Or(u128Shr(a.Lo, n), u128Shl(a.Hi, 128-n)),
	}
}

// Division by an arbitrary divisor uses the method of Möller and Granlund,
// "Improved division by invariant integers" (IEEE Transactions on
// Computers 60(2), 2011): each quotient word is estimated by multiplying by
// a reciprocal of the normalised divisor, computed once per divisor and
// shared by both quotient words of u256Div128, instead of by a 128-by-64
// division per word.
//
// A division is a chain: the digit counts, the scaling, the reciprocal, two
// quotient words and the rounding each wait for the one before, and on
// arm64 (Apple M4 Max) a decimal128 division takes about as long as that
// chain, with little overlap between one division and the next. Two things
// follow, and both were measured (2026-11-10; "Operands without a pattern"
// and "Division" in the package documentation have the figures).
//
// First, a branch on the operands' digits is not free even when it is
// predicted. The corrections of these algorithms depend on low bits of the
// operands: the first correction of the two-word reciprocal applies for
// more than half of all divisors, and that of a quotient word about half
// the time. On operands with no pattern they mispredict, and each
// misprediction discards the division after it as well. They are written
// as carries, borrows and masks.
//
// Second, where bits.Div64 is not an instruction it is a routine of two
// hardware divisions and two correction loops whose trips depend on the
// operand. The reciprocal of a word is computed without it, by a table and
// Newton steps (reciprocalNewton), and a one-word divisor takes two
// reciprocal steps instead of two bits.Div64. On amd64, where bits.Div64 is
// the 128-by-64 division instruction, it computes the reciprocal and
// divides by a one-word divisor directly (div64Native).

// reciprocal2by1 returns ⌊(2^128 - 1) / d⌋ - 2^64, the reciprocal of the
// normalised word d (its top bit set) that div2by1 multiplies by and that
// reciprocalExtend extends: by the division instruction where there is
// one, and by reciprocalNewton elsewhere.
func reciprocal2by1(d uint64) uint64 {
	if div64Native {
		v, _ := bits.Div64(^d, ^uint64(0), d)
		return v
	}
	return reciprocalNewton(d)
}

// recipTab holds ⌊(2^19 - 3×2^8) / d9⌋ for d9 from 256 to 511, the first
// approximation of reciprocal2by1 from the top nine bits of a normalised
// divisor (Möller and Granlund, Algorithm 3, step 1). The values have 11
// bits. TestRecipTab recomputes them.
var recipTab = [256]uint16{
	2045, 2037, 2029, 2021, 2013, 2005, 1998, 1990, 1983, 1975, 1968, 1960,
	1953, 1946, 1938, 1931, 1924, 1917, 1910, 1903, 1896, 1889, 1883, 1876,
	1869, 1863, 1856, 1849, 1843, 1836, 1830, 1824, 1817, 1811, 1805, 1799,
	1792, 1786, 1780, 1774, 1768, 1762, 1756, 1750, 1745, 1739, 1733, 1727,
	1722, 1716, 1710, 1705, 1699, 1694, 1688, 1683, 1677, 1672, 1667, 1661,
	1656, 1651, 1646, 1641, 1636, 1630, 1625, 1620, 1615, 1610, 1605, 1600,
	1596, 1591, 1586, 1581, 1576, 1572, 1567, 1562, 1558, 1553, 1548, 1544,
	1539, 1535, 1530, 1526, 1521, 1517, 1513, 1508, 1504, 1500, 1495, 1491,
	1487, 1483, 1478, 1474, 1470, 1466, 1462, 1458, 1454, 1450, 1446, 1442,
	1438, 1434, 1430, 1426, 1422, 1418, 1414, 1411, 1407, 1403, 1399, 1396,
	1392, 1388, 1384, 1381, 1377, 1374, 1370, 1366, 1363, 1359, 1356, 1352,
	1349, 1345, 1342, 1338, 1335, 1332, 1328, 1325, 1322, 1318, 1315, 1312,
	1308, 1305, 1302, 1299, 1295, 1292, 1289, 1286, 1283, 1280, 1276, 1273,
	1270, 1267, 1264, 1261, 1258, 1255, 1252, 1249, 1246, 1243, 1240, 1237,
	1234, 1231, 1228, 1226, 1223, 1220, 1217, 1214, 1211, 1209, 1206, 1203,
	1200, 1197, 1195, 1192, 1189, 1187, 1184, 1181, 1179, 1176, 1173, 1171,
	1168, 1165, 1163, 1160, 1158, 1155, 1153, 1150, 1148, 1145, 1143, 1140,
	1138, 1135, 1133, 1130, 1128, 1125, 1123, 1121, 1118, 1116, 1113, 1111,
	1109, 1106, 1104, 1102, 1099, 1097, 1095, 1092, 1090, 1088, 1086, 1083,
	1081, 1079, 1077, 1074, 1072, 1070, 1068, 1066, 1064, 1061, 1059, 1057,
	1055, 1053, 1051, 1049, 1047, 1044, 1042, 1040, 1038, 1036, 1034, 1032,
	1030, 1028, 1026, 1024,
}

// reciprocalNewton returns ⌊(2^128 - 1) / d⌋ - 2^64 for a normalised word
// d (its top bit set), as reciprocal2by1 does (Möller and Granlund,
// Algorithm 3). It is that quotient computed without a division: a table gives 11 bits from the top
// nine of d, and Newton steps on the leading 40 bits, on 63 and on all 64
// double them to 21, 34 and 64, the last with an exact correction. With
// d9, d40 and d63 the top 9 and 40 bits of d (the latter plus one) and
// ⌈d / 2⌉:
//
//	v0 = recipTab[d9]
//	v1 = 2^11 v0 - ⌊v0^2 d40 / 2^40⌋ - 1
//	v2 = 2^13 v1 + ⌊v1 (2^60 - v1 d40) / 2^47⌋
//	e  = 2^96 - v2 d63 + ⌊v2 / 2⌋ (d mod 2)
//	v3 = 2^31 v2 + ⌊v2 e / 2^65⌋            (modulo 2^64)
//	v4 = v3 - ⌊(v3 + 2^64 + 1) d / 2^64⌋    (modulo 2^64)
//
// and v4 is the result. Every product fits the words shown in the code;
// the paper proves the bounds. The same value is bits.Div64(^d, ^0, d),
// and TestReciprocalNewton compares the two on every boundary of the table
// and on random divisors, on every target.
func reciprocalNewton(d uint64) uint64 {
	d0 := d & 1
	d40 := d>>24 + 1
	d63 := d>>1 + d0
	v0 := uint64(recipTab[d>>55-256])
	v1 := v0<<11 - v0*v0*d40>>40 - 1
	v2 := v1<<13 + v1*(1<<60-v1*d40)>>47
	e := (v2>>1)&-d0 - v2*d63 // modulo 2^64, where 2^96 is zero
	h, _ := bits.Mul64(v2, e)
	v3 := v2<<31 + h>>1
	h, l := bits.Mul64(v3, d)
	_, c := bits.Add64(l, d, 0)
	return v3 - (h + d + c)
}

// reciprocal3by2 returns ⌊(2^192 - 1) / (d1, d0)⌋ - 2^64, the reciprocal
// of the normalised two-word divisor (d1, d0) that div3by2 multiplies by.
func reciprocal3by2(d1, d0 uint64) uint64 {
	return reciprocalExtend(reciprocal2by1(d1), d1, d0)
}

// reciprocalExtend returns reciprocal3by2(d1, d0) from v, the reciprocal
// of d1 alone (Möller and Granlund, Algorithm 6). The algorithm corrects
// v twice: a decrement when an addition carries, and one more when what
// is left still reaches the divisor. The first carry happens for more
// than half of all divisors, so the corrections are carries and borrows,
// not branches; and the product v d0 that the second correction needs is
// started from the uncorrected v and adjusted by the multiples of d0 the
// first correction took off, so that it does not wait for it.
func reciprocalExtend(v, d1, d0 uint64) uint64 {
	// p = d1 v + d0. If the addition carried: v--, and once more with
	// p -= d1 if p >= d1; then p -= d1.
	p, c := bits.Add64(d1*v, d0, 0)
	_, lt := bits.Sub64(p, d1, 0)
	x := c &^ lt // carried and p >= d1
	// (t1, t0) = (v - c - x) d0.
	t1, t0 := bits.Mul64(v, d0)
	var b uint64
	t0, b = bits.Sub64(t0, d0&-c, 0)
	t1 -= b
	t0, b = bits.Sub64(t0, d0&-x, 0)
	t1 -= b
	v -= c + x
	p -= d1&-c + d1&-x
	// p += t1. If that carried: v--, and once more if (p, t0) >= (d1, d0).
	p, c = bits.Add64(p, t1, 0)
	_, lt = bits.Sub64(t0, d0, 0)
	_, lt = bits.Sub64(p, d1, lt)
	return v - c - c&^lt
}

// div2by1 divides the two-word number (u1, u0) by the normalised word d,
// whose reciprocal v is reciprocal2by1(d), and returns the one-word
// quotient and the remainder (Möller and Granlund, Algorithm 4).
// Precondition, not checked: u1 < d.
//
// The candidate quotient is the high word of v u1 + (u1, u0), plus one;
// the remainder against it, modulo 2^64, exceeds the fraction word q0
// exactly when the candidate is one too large, which on operands with no
// pattern is a coin toss and is corrected by a mask. A last, rare step
// corrects a remainder still at or above the divisor.
func div2by1(u1, u0, d, v uint64) (q, r uint64) {
	q, q0 := bits.Mul64(v, u1)
	q0, c := bits.Add64(q0, u0, 0)
	q += u1 + c + 1
	r = u0 - q*d
	_, gt := bits.Sub64(q0, r, 0) // 1 when r > q0
	m := -gt
	q += m
	r += d & m
	if r >= d {
		q++
		r -= d
	}
	return q, r
}

// div3by2 divides the three-word number (u2, u1, u0) by the two-word number
// (d1, d0), whose reciprocal v is reciprocal3by2(d1, d0), and returns the
// one-word quotient q and the two-word remainder (r1, r0) (Möller and
// Granlund, Algorithm 5).
//
// Preconditions, not checked: d1 has its top bit set (the divisor is
// normalised), and (u2, u1) < (d1, d0), which is exactly the condition for
// the quotient to fit one word.
//
// The candidate quotient (q1, q0) = v u2 + (u2, u1) is the quotient or one
// more than it after the increment below; the remainder computed modulo
// 2^128 against that candidate says which, by comparison with the fraction
// word q0, and adding the divisor back corrects it. A last, rare step
// corrects a remainder that is still at or above the divisor.
func div3by2(u2, u1, u0, d1, d0, v uint64) (q, r1, r0 uint64) {
	q, q0 := bits.Mul64(v, u2)
	q0, c := bits.Add64(q0, u1, 0)
	q += u2 + c
	r1 = u1 - q*d1
	t1, t0 := bits.Mul64(d0, q)
	// (r1, r0) = (r1, u0) - (t1, t0) - (d1, d0), modulo 2^128.
	var b uint64
	r0, b = bits.Sub64(u0, t0, 0)
	r1, _ = bits.Sub64(r1, t1, b)
	r0, b = bits.Sub64(r0, d0, 0)
	r1, _ = bits.Sub64(r1, d1, b)
	// The candidate q + 1 is right or one too large, as r1 compares with
	// the fraction word, and on operands with no pattern that is a coin
	// toss: the correction is a mask, not a branch (the paper says as
	// much). m is all ones when r1 >= q0.
	_, lt := bits.Sub64(r1, q0, 0)
	m := lt - 1
	q += 1 + m
	r0, c = bits.Add64(r0, d0&m, 0)
	r1, _ = bits.Add64(r1, d1&m, c)
	if r1 > d1 || r1 == d1 && r0 >= d0 {
		q++
		r0, b = bits.Sub64(r0, d0, 0)
		r1, _ = bits.Sub64(r1, d1, b)
	}
	return q, r1, r0
}

// div3by2Lead is div3by2 from the reciprocal v of d1 alone: one step of
// Knuth's Algorithm D (TAOCP vol. 2, 4.3.1) with a two-word divisor, its
// estimate taken by div2by1 instead of a division. Preconditions, not
// checked: d1 has its top bit set, and u2 < d1. The quotient word is
// estimated from the top words alone, as q̂ = ⌊(u2·2^64 + u1) / d1⌋;
// because d1 is normalised the estimate is never too small and at most 2
// too large. The low divisor word is multiplied back and subtracted, and
// while that borrows the divisor is added back and q̂ decremented, at most
// twice.
//
// The estimate exceeds the quotient by about q̂ d0 / (d1 2^64), so it is
// right but for a few times in a thousand when q̂ is below 2^55, and wrong
// about one time in three when q̂ is a full word. u256Div128 uses this
// step where the quotient word is small: the leading word of a decimal
// quotient, which has under 50 bits, and there it needs only the
// reciprocal of d1, which is ready before reciprocalExtend's.
func div3by2Lead(u2, u1, u0, d1, d0, v uint64) (q, r1, r0 uint64) {
	q, rt := div2by1(u2, u1, d1, v)

	// (r1, r0) = (rt, u0) - q × d0, with a borrow out of the top.
	h, l := bits.Mul64(q, d0)
	var b, c uint64
	r0, b = bits.Sub64(u0, l, 0)
	r1, b = bits.Sub64(rt, h, b)

	// A borrow means q̂ was too large and the difference wrapped negative.
	// Add the divisor back; the difference is non-negative again exactly
	// when the addition carries out of the top word.
	for b != 0 {
		q--
		r0, c = bits.Add64(r0, d0, 0)
		r1, c = bits.Add64(r1, d1, c)
		b ^= c
	}
	return q, r1, r0
}

// u256Div128 divides n by d when the quotient fits 128 bits.
//
// Precondition, checked: d != 0 and n.Hi < d. That is exactly the condition
// for the quotient to be below 2^128, because n / d < 2^128 if and only if
// n < d × 2^128. ok reports whether it holds. When ok is false the
// quotient does not fit (or d is zero) and q and r are zero; nothing
// panics, and nothing is truncated. A caller whose quotient may need more
// than 128 bits uses u256DivMod128, which returns all of it.
//
// When ok is true, q = ⌊n / d⌋ and r = n - q × d, 0 <= r < d.
func u256Div128(n Uint256, d Uint128) (q, r Uint128, ok bool) {
	if !u128Lt(n.Hi, d) {
		return Uint128{}, Uint128{}, false
	}
	if d.Hi == 0 {
		// n.Hi < d.Lo < 2^64, so n.Hi.Hi is zero and two word divisions
		// suffice: each step's high word is below the divisor, the first
		// by the precondition and the second as a remainder. They are
		// the division instruction where there is one, and elsewhere two
		// steps by the reciprocal of the normalised divisor; the shift
		// cannot lose bits, because n.Hi.Lo < d.Lo.
		if div64Native {
			var r64 uint64
			q.Hi, r64 = bits.Div64(n.Hi.Lo, n.Lo.Hi, d.Lo)
			q.Lo, r64 = bits.Div64(r64, n.Lo.Lo, d.Lo)
			return q, Uint128{Lo: r64}, true
		}
		s := uint(bits.LeadingZeros64(d.Lo))
		dn := d.Lo << s
		v := reciprocal2by1(dn)
		var r64 uint64
		if n.Hi.Lo == 0 {
			// A dividend of two words: the high quotient word is a
			// division of one word by one, the hardware's, which does
			// not wait for the reciprocal; the reciprocal step for the
			// low word takes its remainder, normalised.
			q.Hi = n.Lo.Hi / d.Lo
			r64 = n.Lo.Hi - q.Hi*d.Lo
			q.Lo, r64 = div2by1(r64<<s|n.Lo.Lo>>(64-s), n.Lo.Lo<<s, dn, v)
			return q, Uint128{Lo: r64 >> s}, true
		}
		q.Hi, r64 = div2by1(n.Hi.Lo<<s|n.Lo.Hi>>(64-s), n.Lo.Hi<<s|n.Lo.Lo>>(64-s), dn, v)
		q.Lo, r64 = div2by1(r64, n.Lo.Lo<<s, dn, v)
		return q, Uint128{Lo: r64 >> s}, true
	}

	// Normalise: shift d left until its top bit is set, and n by the same
	// amount. n << s cannot lose bits because n.Hi < d implies
	// n.Hi << s < d << s < 2^128. For s == 0 the shifts by 64-s are by 64,
	// which yields zero, so no case analysis is needed.
	s := uint(bits.LeadingZeros64(d.Hi))
	v1 := d.Hi<<s | d.Lo>>(64-s)
	v0 := d.Lo << s
	w3 := n.Hi.Hi<<s | n.Hi.Lo>>(64-s)
	w2 := n.Hi.Lo<<s | n.Lo.Hi>>(64-s)
	w1 := n.Lo.Hi<<s | n.Lo.Lo>>(64-s)
	w0 := n.Lo.Lo << s

	// Two quotient words, high first. The first step's precondition
	// (w3, w2) < (v1, v0) is n.Hi < d after the shift; the second's is that
	// the remainder of the first is below the divisor.
	//
	// A dividend below 2^128 has a high quotient word of zero, because
	// d >= 2^64, and only the low word to find, by the step that needs the
	// reciprocal of v1 alone. Otherwise the low word takes the two-word
	// reciprocal, and so does the high word unless it is small: w3 below
	// 2^55 bounds it below 2^56, and then the step from the reciprocal of
	// v1 is right but for a few times in a thousand and does not wait for
	// the two-word reciprocal, which is extended beside it. The high word
	// of a decimal quotient is always that small (under 50 bits for 34
	// digits); the test is on the operands' magnitude, not their digits.
	v := reciprocal2by1(v1)
	var r1, r0 uint64
	if n.Hi == (Uint128{}) {
		q.Lo, r1, r0 = div3by2Lead(w2, w1, w0, v1, v0, v)
	} else {
		vv := reciprocalExtend(v, v1, v0)
		if w3>>55 == 0 {
			q.Hi, r1, r0 = div3by2Lead(w3, w2, w1, v1, v0, v)
		} else {
			q.Hi, r1, r0 = div3by2(w3, w2, w1, v1, v0, vv)
		}
		q.Lo, r1, r0 = div3by2(r1, r0, w0, v1, v0, vv)
	}

	// Undo the normalisation of the remainder.
	r = Uint128{Hi: r1 >> s, Lo: r0>>s | r1<<(64-s)}
	return q, r, true
}

// divisor128 is a non-zero divisor prepared for repeated division: its
// normalised words and their reciprocals, which a division would compute
// again each time. The remainder of a long exponentiation divides by one
// modulus some twenty times.
type divisor128 struct {
	d      Uint128
	v1, v0 uint64 // d shifted left by s: both words, or v1 alone for a one-word d
	v      uint64 // reciprocal2by1(v1)
	vv     uint64 // reciprocal3by2(v1, v0), for a two-word d
	s      uint
}

// prepareDivisor128 returns d prepared. Precondition: d is not zero.
func prepareDivisor128(d Uint128) divisor128 {
	if d.Hi == 0 {
		s := uint(bits.LeadingZeros64(d.Lo))
		v1 := d.Lo << s
		return divisor128{d: d, v1: v1, v: reciprocal2by1(v1), s: s}
	}
	s := uint(bits.LeadingZeros64(d.Hi))
	v1 := d.Hi<<s | d.Lo>>(64-s)
	v0 := d.Lo << s
	v := reciprocal2by1(v1)
	return divisor128{d: d, v1: v1, v0: v0, v: v, vv: reciprocalExtend(v, v1, v0), s: s}
}

// u256ModPrepared returns n mod p.d for an n whose quotient fits 128 bits,
// that is, n.Hi < p.d: the steps of u256Div128 with the reciprocals at
// hand and the quotient words discarded. Precondition, not checked:
// n.Hi < p.d.
func u256ModPrepared(n Uint256, p *divisor128) Uint128 {
	s := p.s
	if p.d.Hi == 0 {
		if div64Native {
			_, r := bits.Div64(n.Hi.Lo, n.Lo.Hi, p.d.Lo)
			_, r = bits.Div64(r, n.Lo.Lo, p.d.Lo)
			return Uint128{Lo: r}
		}
		_, r := div2by1(n.Hi.Lo<<s|n.Lo.Hi>>(64-s), n.Lo.Hi<<s|n.Lo.Lo>>(64-s), p.v1, p.v)
		_, r = div2by1(r, n.Lo.Lo<<s, p.v1, p.v)
		return Uint128{Lo: r >> s}
	}
	w3 := n.Hi.Hi<<s | n.Hi.Lo>>(64-s)
	w2 := n.Hi.Lo<<s | n.Lo.Hi>>(64-s)
	w1 := n.Lo.Hi<<s | n.Lo.Lo>>(64-s)
	w0 := n.Lo.Lo << s
	_, r1, r0 := div3by2(w3, w2, w1, p.v1, p.v0, p.vv)
	_, r1, r0 = div3by2(r1, r0, w0, p.v1, p.v0, p.vv)
	return Uint128{Hi: r1 >> s, Lo: r0>>s | r1<<(64-s)}
}

// u256DivMod128 returns the full quotient and the remainder of n / d, with
// 0 <= r < d. The quotient is up to 256 bits and nothing is discarded: this
// is the primitive for a caller that cannot bound the quotient by 2^128.
// Precondition: d != 0; a zero d panics with errDivideByZero.
//
// It is long division in base 2^128: the high half of n divides first
// (its quotient is the high half of the result, with a remainder below d),
// and that remainder joined to the low half of n divides second, which
// satisfies u256Div128's precondition by construction.
func u256DivMod128(n Uint256, d Uint128) (q Uint256, r Uint128) {
	if d == (Uint128{}) {
		panic(errDivideByZero)
	}
	var rh Uint128
	q.Hi, rh = u128Div(n.Hi, d)
	q.Lo, r, _ = u256Div128(Uint256{Hi: rh, Lo: n.Lo}, d)
	return q, r
}
