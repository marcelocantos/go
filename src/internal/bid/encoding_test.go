// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Tests of the BID codec and the encoding predicates of encoding.go.
//
// They are written from the layout of IEEE 754-2008 clause 3.5.2 alone. The
// decoders and encoders below split an encoding into the sign S, the
// combination field G (13 bits for decimal64, 17 for decimal128) and the
// trailing significand field T (50 and 110 bits), and read the standard's
// rules off G bit by bit. They share no code, no constant and no
// formulation with the kernel or with the reference model of ref_test.go,
// which splits the encoding at the exponent field instead.

import (
	"fmt"
	"internal/bid"
	"math"
	"math/big"
	"testing"
)

// ieee is a decoded encoding, with the facts the predicates need.
type ieee struct {
	kind      bid.Kind
	neg       bool
	signaling bool
	canonical bool
	coeff     bid.Uint128 // zero when the stored coefficient is above 10^p-1
	exp       int
}

const (
	// Largest coefficient of each format, 10^16-1 and 10^34-1.
	ieeeMax64   = 9999999999999999
	ieeeMax128H = 0x1ED09BEAD87C0
	ieeeMax128L = 0x378D8E63FFFFFFFF
	// Smallest NaN payload that is not canonical, 10^15 and 10^33.
	ieeeNaNLimit64   = 1_000_000_000_000_000
	ieeeNaNLimit128H = 0x314DC6448D93
	ieeeNaNLimit128L = 0x38C15B0A00000000
)

// ieeeDecode64 decodes a decimal64 per IEEE 754-2008 clause 3.5.2.
func ieeeDecode64(x uint64) ieee {
	s := x >> 63
	g := x >> 50 & 0x1FFF // G0 is bit 12 of g, G12 is bit 0
	t := x & (1<<50 - 1)
	v := ieee{neg: s == 1, canonical: true}
	switch {
	case g>>8 == 0b11110: // G0..G4: infinity
		v.kind = bid.KindInf
		v.canonical = g&0xFF == 0 && t == 0 // G5..G12 and T are zero
	case g>>8 == 0b11111: // G0..G4: NaN
		v.kind = bid.KindNaN
		v.signaling = g>>7&1 == 1                       // G5
		v.canonical = g&0x7F == 0 && t < ieeeNaNLimit64 // G6..G12 zero
	default:
		var e, c uint64
		if g>>11 == 0b11 { // G0G1 = 11: the significand starts 100 or 1001
			e = g >> 1 & 0x3FF // G2..G11
			c = (8+g&1)<<50 | t
		} else {
			e = g >> 3 // G0..G9
			c = (g&7)<<50 | t
		}
		if c > ieeeMax64 {
			v.canonical = false
			c = 0
		}
		v.coeff = bid.Uint128{Lo: c}
		v.exp = int(e) - 398
	}
	return v
}

// ieeeDecode128 decodes a decimal128 per IEEE 754-2008 clause 3.5.2.
func ieeeDecode128(x bid.Uint128) ieee {
	s := x.Hi >> 63
	g := x.Hi >> 46 & 0x1FFFF // G0 is bit 16 of g, G16 is bit 0
	tHi := x.Hi & (1<<46 - 1) // T is tHi:x.Lo
	v := ieee{neg: s == 1, canonical: true}
	switch {
	case g>>12 == 0b11110: // G0..G4: infinity
		v.kind = bid.KindInf
		v.canonical = g&0xFFF == 0 && tHi == 0 && x.Lo == 0
	case g>>12 == 0b11111: // G0..G4: NaN
		v.kind = bid.KindNaN
		v.signaling = g>>11&1 == 1 // G5
		below := tHi < ieeeNaNLimit128H || tHi == ieeeNaNLimit128H && x.Lo < ieeeNaNLimit128L
		v.canonical = g&0x7FF == 0 && below // G6..G16 zero
	case g>>15 == 0b11: // G0G1 = 11: the significand starts 100, so it is at least 2^113
		v.canonical = false
		v.exp = int(g>>1&0x3FFF) - 6176 // G2..G15
	default:
		v.exp = int(g>>3) - 6176 // G0..G13
		c := bid.Uint128{Hi: (g&7)<<46 | tHi, Lo: x.Lo}
		if c.Hi > ieeeMax128H || c.Hi == ieeeMax128H && c.Lo > ieeeMax128L {
			v.canonical = false
			c = bid.Uint128{}
		}
		v.coeff = c
	}
	return v
}

// ieeeEncode64 encodes a finite decimal64 in the canonical form of clause
// 3.5.2. The arguments must be in range.
func ieeeEncode64(neg bool, coeff uint64, exp int) uint64 {
	e := uint64(exp + 398)
	var g, t uint64
	if coeff < 1<<53 { // the three leading bits of the coefficient are not 100 or 101
		g = e<<3 | coeff>>50
	} else { // 1000 or 1001 leading: G0G1 = 11 and G12 is the fourth bit
		g = 0b11<<11 | e<<1 | coeff>>50&1
	}
	t = coeff & (1<<50 - 1)
	x := g<<50 | t
	if neg {
		x |= 1 << 63
	}
	return x
}

// ieeeEncode128 is ieeeEncode64 for decimal128. A canonical coefficient is
// below 2^113, so the second form never arises.
func ieeeEncode128(neg bool, coeff bid.Uint128, exp int) bid.Uint128 {
	e := uint64(exp + 6176)
	g := e<<3 | coeff.Hi>>46&7
	x := bid.Uint128{Hi: g<<46 | coeff.Hi&(1<<46-1), Lo: coeff.Lo}
	if neg {
		x.Hi |= 1 << 63
	}
	return x
}

func hex128(x bid.Uint128) string { return fmt.Sprintf("%016x%016x", x.Hi, x.Lo) }

// checkDecode64 checks every decoding function against the IEEE decoder
// and the round trip through Pack64. It is the body of the structured
// sweeps and of the fuzz target.
func checkDecode64(t *testing.T, x uint64) {
	t.Helper()
	w := ieeeDecode64(x)
	kind, neg, coeff, exp := bid.Unpack64(x)
	if kind != w.kind || neg != w.neg || coeff != w.coeff.Lo || exp != w.exp {
		t.Fatalf("Unpack64(%016x) = (%d, %t, %d, %d), want (%d, %t, %d, %d)",
			x, kind, neg, coeff, exp, w.kind, w.neg, w.coeff.Lo, w.exp)
	}
	if got := bid.IsNaN64(x); got != (w.kind == bid.KindNaN) {
		t.Fatalf("IsNaN64(%016x) = %t", x, got)
	}
	if got := bid.IsSignaling64(x); got != (w.kind == bid.KindNaN && w.signaling) {
		t.Fatalf("IsSignaling64(%016x) = %t", x, got)
	}
	if got := bid.IsInf64(x); got != (w.kind == bid.KindInf) {
		t.Fatalf("IsInf64(%016x) = %t", x, got)
	}
	if got := bid.IsFinite64(x); got != (w.kind == bid.KindFinite) {
		t.Fatalf("IsFinite64(%016x) = %t", x, got)
	}
	if got := bid.IsZero64(x); got != (w.kind == bid.KindFinite && w.coeff.Lo == 0) {
		t.Fatalf("IsZero64(%016x) = %t", x, got)
	}
	if got := bid.IsCanonical64(x); got != w.canonical {
		t.Fatalf("IsCanonical64(%016x) = %t, want %t", x, got, w.canonical)
	}
	if got := bid.Signbit64(x); got != w.neg {
		t.Fatalf("Signbit64(%016x) = %t", x, got)
	}
	if w.kind != bid.KindFinite {
		return
	}
	wantSub := false
	if w.coeff.Lo != 0 {
		n := len(fmt.Sprint(w.coeff.Lo))
		wantSub = n-1+w.exp < -383
	}
	if got := bid.IsSubnormal64(x); got != wantSub {
		t.Fatalf("IsSubnormal64(%016x) = %t, want %t", x, got, wantSub)
	}
	// The two bits below the sign never make the exponent field exceed
	// 767 (3 * 2^8 - 1) in either form, so the exponent is always in range.
	if exp < -398 || exp > 369 {
		t.Fatalf("Unpack64(%016x) exponent %d is out of range", x, exp)
	}
	// Re-encoding gives the canonical form of the value read. A canonical
	// encoding is its own canonical form.
	y, ok := bid.Pack64(neg, coeff, exp)
	if !ok || y != ieeeEncode64(neg, coeff, exp) || !bid.IsCanonical64(y) {
		t.Fatalf("Pack64(Unpack64(%016x)) = %016x, %t", x, y, ok)
	}
	if w.canonical && y != x {
		t.Fatalf("Pack64(Unpack64(%016x)) = %016x on a canonical encoding", x, y)
	}
	if !w.canonical && coeff != 0 {
		t.Fatalf("non-canonical %016x read as non-zero %d", x, coeff)
	}
}

// checkDecode128 is checkDecode64 for decimal128.
func checkDecode128(t *testing.T, x bid.Uint128) {
	t.Helper()
	w := ieeeDecode128(x)
	kind, neg, coeff, exp := bid.Unpack128(x)
	if kind != w.kind || neg != w.neg || coeff != w.coeff || exp != w.exp {
		t.Fatalf("Unpack128(%s) = (%d, %t, %s, %d), want (%d, %t, %s, %d)",
			hex128(x), kind, neg, hex128(coeff), exp, w.kind, w.neg, hex128(w.coeff), w.exp)
	}
	if got := bid.IsNaN128(x); got != (w.kind == bid.KindNaN) {
		t.Fatalf("IsNaN128(%s) = %t", hex128(x), got)
	}
	if got := bid.IsSignaling128(x); got != (w.kind == bid.KindNaN && w.signaling) {
		t.Fatalf("IsSignaling128(%s) = %t", hex128(x), got)
	}
	if got := bid.IsInf128(x); got != (w.kind == bid.KindInf) {
		t.Fatalf("IsInf128(%s) = %t", hex128(x), got)
	}
	if got := bid.IsFinite128(x); got != (w.kind == bid.KindFinite) {
		t.Fatalf("IsFinite128(%s) = %t", hex128(x), got)
	}
	zero := w.coeff == bid.Uint128{}
	if got := bid.IsZero128(x); got != (w.kind == bid.KindFinite && zero) {
		t.Fatalf("IsZero128(%s) = %t", hex128(x), got)
	}
	if got := bid.IsCanonical128(x); got != w.canonical {
		t.Fatalf("IsCanonical128(%s) = %t, want %t", hex128(x), got, w.canonical)
	}
	if got := bid.Signbit128(x); got != w.neg {
		t.Fatalf("Signbit128(%s) = %t", hex128(x), got)
	}
	if w.kind != bid.KindFinite {
		return
	}
	wantSub := false
	if !zero {
		n := len(fromUint128(w.coeff).String())
		wantSub = n-1+w.exp < -6143
	}
	if got := bid.IsSubnormal128(x); got != wantSub {
		t.Fatalf("IsSubnormal128(%s) = %t, want %t", hex128(x), got, wantSub)
	}
	if exp < -6176 || exp > 6111 { // exponent field at most 3 * 2^12 - 1 in either form
		t.Fatalf("Unpack128(%s) exponent %d is out of range", hex128(x), exp)
	}
	y, ok := bid.Pack128(neg, coeff, exp)
	if !ok || y != ieeeEncode128(neg, coeff, exp) || !bid.IsCanonical128(y) {
		t.Fatalf("Pack128(Unpack128(%s)) = %s, %t", hex128(x), hex128(y), ok)
	}
	if w.canonical && y != x {
		t.Fatalf("Pack128(Unpack128(%s)) = %s on a canonical encoding", hex128(x), hex128(y))
	}
	if !w.canonical && !zero {
		t.Fatalf("non-canonical %s read as non-zero %s", hex128(x), hex128(coeff))
	}
}

// trailing64 holds the T patterns the combination-field sweep pairs with
// every G: the boundaries of the coefficient range and of the NaN payload
// range, all ones, and alternating bits.
var trailing64 = []uint64{
	0, 1, 1<<50 - 1, 1 << 49,
	ieeeNaNLimit64 - 1, ieeeNaNLimit64, 0x2AAAAAAAAAAAA, 0x1555555555555,
	// The tail of the coefficient range in the second form: 10^16-1 is
	// 2^53 + 992800745259007.
	992800745259006, 992800745259007, 992800745259008,
}

var trailing128 = []bid.Uint128{
	{}, {Lo: 1}, {Hi: 1<<46 - 1, Lo: ^uint64(0)}, {Hi: 1 << 45},
	{Hi: ieeeNaNLimit128H, Lo: ieeeNaNLimit128L - 1}, {Hi: ieeeNaNLimit128H, Lo: ieeeNaNLimit128L},
	{Hi: 0x2AAAAAAAAAAA, Lo: 0xAAAAAAAAAAAAAAAA}, {Hi: 0x155555555555, Lo: 0x5555555555555555},
	{Hi: ieeeMax128H & (1<<46 - 1), Lo: ieeeMax128L},
}

// Every combination-field prefix of decimal64: all 8192 values of G, both
// signs, with the trailing patterns above. This covers the three ordinary
// prefixes 00, 01 and 10, the large-coefficient prefix 11, 11110
// (infinity), 111110 (quiet NaN) and 111111 (signaling NaN), with every
// value of the reserved bits.
func TestEncodingSweepCombination64(t *testing.T) {
	for _, sign := range []uint64{0, 1 << 63} {
		for g := uint64(0); g < 1<<13; g++ {
			for _, tr := range trailing64 {
				checkDecode64(t, sign|g<<50|tr)
			}
		}
	}
}

// Every combination-field prefix of decimal128: all 131072 values of G.
func TestEncodingSweepCombination128(t *testing.T) {
	for _, sign := range []uint64{0, 1 << 63} {
		for g := uint64(0); g < 1<<17; g++ {
			for _, tr := range trailing128 {
				checkDecode128(t, bid.Uint128{Hi: sign | g<<46 | tr.Hi, Lo: tr.Lo})
			}
		}
	}
}

// Every single-bit encoding and its complement, which catches a field
// boundary placed one bit off.
func TestEncodingSweepSingleBits(t *testing.T) {
	for i := 0; i < 64; i++ {
		checkDecode64(t, 1<<i)
		checkDecode64(t, ^uint64(1<<i))
	}
	for i := 0; i < 128; i++ {
		var one bid.Uint128
		if i < 64 {
			one.Lo = 1 << i
		} else {
			one.Hi = 1 << (i - 64)
		}
		checkDecode128(t, one)
		checkDecode128(t, bid.Uint128{Hi: ^one.Hi, Lo: ^one.Lo})
	}
}

// The expected fields below are written out literally, one per case, so
// that a failure names the case and no helper stands between the table and
// the kernel.
func TestUnpack64Table(t *testing.T) {
	const (
		neg   = uint64(1) << 63
		max   = 9999999999999999
		zeroE = uint64(398) << 53 // biased exponent 398: exponent 0, first form
	)
	cases := []struct {
		name  string
		x     uint64
		kind  bid.Kind
		neg   bool
		coeff uint64
		exp   int
	}{
		{"+0", zeroE, bid.KindFinite, false, 0, 0},
		{"-0", neg | zeroE, bid.KindFinite, true, 0, 0},
		{"+1", zeroE | 1, bid.KindFinite, false, 1, 0},
		{"-1", neg | zeroE | 1, bid.KindFinite, true, 1, 0},
		{"smallest subnormal", 1, bid.KindFinite, false, 1, -398},
		{"first form, largest coefficient 2^53-1", zeroE | (1<<53 - 1), bid.KindFinite, false, 1<<53 - 1, 0},
		{"first form, exponent field 0", 5, bid.KindFinite, false, 5, -398},
		{"first form, exponent field 767", 767<<53 | 5, bid.KindFinite, false, 5, 369},
		{"first form, exponent field 0x100 (prefix 01)", 0x100<<53 | 5, bid.KindFinite, false, 5, 256 - 398},
		{"first form, exponent field 0x200 (prefix 10)", 0x200<<53 | 5, bid.KindFinite, false, 5, 512 - 398},
		{"second form, coefficient 2^53", 3<<61 | 398<<51, bid.KindFinite, false, 1 << 53, 0},
		{"second form, coefficient 2^53+1", 3<<61 | 398<<51 | 1, bid.KindFinite, false, 1<<53 + 1, 0},
		{"second form, coefficient 10^16-1", 3<<61 | 398<<51 | 992800745259007, bid.KindFinite, false, max, 0},
		{"second form, coefficient 10^16", 3<<61 | 398<<51 | 992800745259008, bid.KindFinite, false, 0, 0},
		{"second form, largest field", neg | 3<<61 | 398<<51 | (1<<51 - 1), bid.KindFinite, true, 0, 0},
		{"second form, exponent field 0", 3<<61 | 992800745259007, bid.KindFinite, false, max, -398},
		{"second form, exponent field 767", 3<<61 | 767<<51 | 992800745259007, bid.KindFinite, false, max, 369},
		{"second form, non-canonical, exponent field 767", neg | 3<<61 | 767<<51 | (1<<51 - 1), bid.KindFinite, true, 0, 369},
		{"second form, non-canonical, exponent field 256", 3<<61 | 256<<51 | 1<<50, bid.KindFinite, false, 0, 256 - 398},
		{"second form, G12 set, non-canonical", 3<<61 | 398<<51 | 1<<50, bid.KindFinite, false, 0, 0},
		{"+Inf", 0x7800000000000000, bid.KindInf, false, 0, 0},
		{"-Inf", 0xF800000000000000, bid.KindInf, true, 0, 0},
		{"Inf with reserved bits", 0x7BFFFFFFFFFFFFFF, bid.KindInf, false, 0, 0},
		{"qNaN", 0x7C00000000000000, bid.KindNaN, false, 0, 0},
		{"-qNaN", 0xFC00000000000000, bid.KindNaN, true, 0, 0},
		{"sNaN", 0x7E00000000000000, bid.KindNaN, false, 0, 0},
		{"-sNaN", 0xFE00000000000000, bid.KindNaN, true, 0, 0},
		{"qNaN with payload and reserved bits", 0x7DFFFFFFFFFFFFFF, bid.KindNaN, false, 0, 0},
		{"sNaN with payload and reserved bits", 0x7FFFFFFFFFFFFFFF, bid.KindNaN, false, 0, 0},
	}
	for _, c := range cases {
		kind, n, coeff, exp := bid.Unpack64(c.x)
		if kind != c.kind || n != c.neg || coeff != c.coeff || exp != c.exp {
			t.Errorf("%s: Unpack64(%016x) = (%d, %t, %d, %d), want (%d, %t, %d, %d)",
				c.name, c.x, kind, n, coeff, exp, c.kind, c.neg, c.coeff, c.exp)
		}
		checkDecode64(t, c.x)
	}
}

func TestUnpack128Table(t *testing.T) {
	const neg = uint64(1) << 63
	// 10^34-1 and 10^34 as words.
	max := bid.Uint128{Hi: 0x1ED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF}
	maxPlus1 := bid.Uint128{Hi: 0x1ED09BEAD87C0, Lo: 0x378D8E6400000000}
	const zeroE = uint64(6176) << 49 // exponent 0, first form
	cases := []struct {
		name  string
		x     bid.Uint128
		kind  bid.Kind
		neg   bool
		coeff bid.Uint128
		exp   int
	}{
		{"+0", bid.Uint128{Hi: zeroE}, bid.KindFinite, false, bid.Uint128{}, 0},
		{"-0", bid.Uint128{Hi: neg | zeroE}, bid.KindFinite, true, bid.Uint128{}, 0},
		{"+1", bid.Uint128{Hi: zeroE, Lo: 1}, bid.KindFinite, false, bid.Uint128{Lo: 1}, 0},
		{"smallest subnormal", bid.Uint128{Lo: 1}, bid.KindFinite, false, bid.Uint128{Lo: 1}, -6176},
		{"coefficient 2^64", bid.Uint128{Hi: zeroE | 1}, bid.KindFinite, false, bid.Uint128{Hi: 1}, 0},
		{"coefficient 10^34-1", bid.Uint128{Hi: zeroE | max.Hi, Lo: max.Lo}, bid.KindFinite, false, max, 0},
		{"-(10^34-1) at the largest exponent", bid.Uint128{Hi: neg | 12287<<49 | max.Hi, Lo: max.Lo}, bid.KindFinite, true, max, 6111},
		{"coefficient 10^34", bid.Uint128{Hi: zeroE | maxPlus1.Hi, Lo: maxPlus1.Lo}, bid.KindFinite, false, bid.Uint128{}, 0},
		{"coefficient 2^113-1", bid.Uint128{Hi: neg | zeroE | (1<<49 - 1), Lo: ^uint64(0)}, bid.KindFinite, true, bid.Uint128{}, 0},
		{"non-canonical zero keeps its exponent", bid.Uint128{Hi: 12287<<49 | (1<<49 - 1), Lo: ^uint64(0)}, bid.KindFinite, false, bid.Uint128{}, 6111},
		{"second form, G16 set, largest exponent", bid.Uint128{Hi: 3<<61 | 12287<<47 | 1<<46}, bid.KindFinite, false, bid.Uint128{}, 6111},
		{"first form, exponent field 0x1000 (prefix 01)", bid.Uint128{Hi: 0x1000<<49 | 7, Lo: 0}, bid.KindFinite, false, bid.Uint128{Hi: 7}, 4096 - 6176},
		{"first form, exponent field 0x2000 (prefix 10)", bid.Uint128{Hi: 0x2000<<49 | 7, Lo: 0}, bid.KindFinite, false, bid.Uint128{Hi: 7}, 8192 - 6176},
		{"second form, zero coefficient field", bid.Uint128{Hi: 3<<61 | 6176<<47}, bid.KindFinite, false, bid.Uint128{}, 0},
		{"second form, all coefficient bits", bid.Uint128{Hi: neg | 3<<61 | 6176<<47 | (1<<47 - 1), Lo: ^uint64(0)}, bid.KindFinite, true, bid.Uint128{}, 0},
		{"second form, exponent field 0", bid.Uint128{Hi: 3 << 61}, bid.KindFinite, false, bid.Uint128{}, -6176},
		{"second form, exponent field 12287", bid.Uint128{Hi: 3<<61 | 12287<<47}, bid.KindFinite, false, bid.Uint128{}, 6111},
		{"second form, exponent field 0x1000", bid.Uint128{Hi: neg | 3<<61 | 0x1000<<47}, bid.KindFinite, true, bid.Uint128{}, 4096 - 6176},
		{"+Inf", bid.Uint128{Hi: 0x7800000000000000}, bid.KindInf, false, bid.Uint128{}, 0},
		{"-Inf", bid.Uint128{Hi: 0xF800000000000000}, bid.KindInf, true, bid.Uint128{}, 0},
		{"Inf with reserved bits", bid.Uint128{Hi: 0x7BFFFFFFFFFFFFFF, Lo: ^uint64(0)}, bid.KindInf, false, bid.Uint128{}, 0},
		{"qNaN", bid.Uint128{Hi: 0x7C00000000000000}, bid.KindNaN, false, bid.Uint128{}, 0},
		{"-sNaN", bid.Uint128{Hi: 0xFE00000000000000}, bid.KindNaN, true, bid.Uint128{}, 0},
		{"qNaN with payload and reserved bits", bid.Uint128{Hi: 0x7DFFFFFFFFFFFFFF, Lo: ^uint64(0)}, bid.KindNaN, false, bid.Uint128{}, 0},
		{"sNaN with payload and reserved bits", bid.Uint128{Hi: 0x7FFFFFFFFFFFFFFF, Lo: ^uint64(0)}, bid.KindNaN, false, bid.Uint128{}, 0},
	}
	for _, c := range cases {
		kind, n, coeff, exp := bid.Unpack128(c.x)
		if kind != c.kind || n != c.neg || coeff != c.coeff || exp != c.exp {
			t.Errorf("%s: Unpack128(%s) = (%d, %t, %s, %d), want (%d, %t, %s, %d)",
				c.name, hex128(c.x), kind, n, hex128(coeff), exp, c.kind, c.neg, hex128(c.coeff), c.exp)
		}
		checkDecode128(t, c.x)
	}
}

// The canonical special values, with the exact bit patterns of the
// standard: the bits below the five-bit prefix 11110 or 11111 are zero but
// for the signaling bit.
func TestCanonicalSpecials(t *testing.T) {
	if bid.Inf64 != 0x7800000000000000 || bid.NaN64 != 0x7C00000000000000 || bid.SNaN64 != 0x7E00000000000000 ||
		bid.Inf128Hi != 0x7800000000000000 || bid.NaN128Hi != 0x7C00000000000000 || bid.SNaN128Hi != 0x7E00000000000000 {
		t.Errorf("special constants: %x %x %x / %x %x %x", bid.Inf64, bid.NaN64, bid.SNaN64, bid.Inf128Hi, bid.NaN128Hi, bid.SNaN128Hi)
	}
	if bid.Zero64 != 0x31C0000000000000 || bid.Zero128Hi != 0x3040000000000000 {
		t.Errorf("zero constants: %x / %x", bid.Zero64, bid.Zero128Hi)
	}
	if !bid.IsCanonical64(bid.Inf64|bid.SignMask64) || !bid.IsCanonical64(bid.NaN64) || !bid.IsCanonical64(bid.SNaN64) {
		t.Error("a canonical decimal64 special is not canonical")
	}
	if !bid.IsCanonical128(bid.Uint128{Hi: bid.Inf128Hi | bid.SignMask128Hi}) || !bid.IsCanonical128(bid.Uint128{Hi: bid.NaN128Hi}) ||
		!bid.IsCanonical128(bid.Uint128{Hi: bid.SNaN128Hi}) {
		t.Error("a canonical decimal128 special is not canonical")
	}
}

// NaN canonicity: reserved bits zero and payload below 10^(p-1). The
// payload and the sign are not part of a result's value (the contract
// reads and produces no NaN payload), but IsCanonical reports them.
func TestNaNPayloadCanonicity(t *testing.T) {
	const (
		q64 = uint64(0x7C00000000000000)
		s64 = uint64(0x7E00000000000000)
	)
	cases64 := []struct {
		name string
		x    uint64
		want bool
	}{
		{"qNaN, payload 0", q64, true},
		{"qNaN, payload 1", q64 | 1, true},
		{"qNaN, payload 10^15-1", q64 | 999999999999999, true},
		{"qNaN, payload 10^15", q64 | 1000000000000000, false},
		{"qNaN, payload 2^50-1", q64 | (1<<50 - 1), false},
		{"sNaN, payload 10^15-1", s64 | 999999999999999, true},
		{"sNaN, payload 10^15", s64 | 1000000000000000, false},
		{"-qNaN, payload 5", 1<<63 | q64 | 5, true},
		{"qNaN, lowest reserved bit (50)", q64 | 1<<50, false},
		{"qNaN, highest reserved bit (56)", q64 | 1<<56, false},
		{"sNaN, bit 56", s64 | 1<<56, false},
		{"+Inf", 0x7800000000000000, true},
		{"Inf, bit 0", 0x7800000000000001, false},
		{"Inf, bit 57", 0x7A00000000000000, false},
		{"-Inf", 0xF800000000000000, true},
	}
	for _, c := range cases64 {
		if got := bid.IsCanonical64(c.x); got != c.want {
			t.Errorf("IsCanonical64(%016x) %s = %t, want %t", c.x, c.name, got, c.want)
		}
		checkDecode64(t, c.x)
	}

	const (
		qHi = uint64(0x7C00000000000000)
		sHi = uint64(0x7E00000000000000)
	)
	// 10^33 = 0x314DC6448D93_38C15B0A00000000.
	cases128 := []struct {
		name string
		x    bid.Uint128
		want bool
	}{
		{"qNaN, payload 0", bid.Uint128{Hi: qHi}, true},
		{"qNaN, payload 10^33-1", bid.Uint128{Hi: qHi | 0x314DC6448D93, Lo: 0x38C15B09FFFFFFFF}, true},
		{"qNaN, payload 10^33", bid.Uint128{Hi: qHi | 0x314DC6448D93, Lo: 0x38C15B0A00000000}, false},
		{"sNaN, payload 10^33-1", bid.Uint128{Hi: sHi | 0x314DC6448D93, Lo: 0x38C15B09FFFFFFFF}, true},
		{"sNaN, payload 10^33", bid.Uint128{Hi: sHi | 0x314DC6448D93, Lo: 0x38C15B0A00000000}, false},
		{"qNaN, payload 2^110-1", bid.Uint128{Hi: qHi | (1<<46 - 1), Lo: ^uint64(0)}, false},
		{"qNaN, lowest reserved bit (110)", bid.Uint128{Hi: qHi | 1<<46}, false},
		{"qNaN, highest reserved bit (120)", bid.Uint128{Hi: qHi | 1<<56}, false},
		{"+Inf", bid.Uint128{Hi: 0x7800000000000000}, true},
		{"Inf, Lo bit 0", bid.Uint128{Hi: 0x7800000000000000, Lo: 1}, false},
		{"Inf, bit 121", bid.Uint128{Hi: 0x7A00000000000000}, false},
	}
	for _, c := range cases128 {
		if got := bid.IsCanonical128(c.x); got != c.want {
			t.Errorf("IsCanonical128(%s) %s = %t, want %t", hex128(c.x), c.name, got, c.want)
		}
		checkDecode128(t, c.x)
	}
}

// Boundaries of IsCanonical and IsSubnormal on finite encodings.
func TestFiniteBoundaries(t *testing.T) {
	// decimal64: 10^16-1 canonical, 10^16 not; the first form never holds
	// a non-canonical coefficient.
	if !bid.IsCanonical64(3<<61|398<<51|992800745259007) || bid.IsCanonical64(3<<61|398<<51|992800745259008) {
		t.Error("IsCanonical64 misplaces the 10^16 boundary")
	}
	if !bid.IsCanonical64(398<<53 | (1<<53 - 1)) {
		t.Error("IsCanonical64 rejects the largest first-form coefficient")
	}
	// decimal128: 10^34-1 canonical, 10^34 not, every second form not.
	if !bid.IsCanonical128(bid.Uint128{Hi: 6176<<49 | 0x1ED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF}) ||
		bid.IsCanonical128(bid.Uint128{Hi: 6176<<49 | 0x1ED09BEAD87C0, Lo: 0x378D8E6400000000}) ||
		bid.IsCanonical128(bid.Uint128{Hi: 3 << 61}) {
		t.Error("IsCanonical128 misplaces the 10^34 boundary")
	}

	sub64 := []struct {
		coeff uint64
		exp   int
		want  bool
	}{
		{1, -383, false}, // 1E-383 is the smallest normal
		{1, -384, true},
		{1, -398, true},
		{999999999999999, -398, true},   // 9.99...E-384
		{1000000000000000, -398, false}, // 1.000...E-383
		{9999999999999999, -398, false},
		{0, -398, false},
	}
	for _, c := range sub64 {
		x, ok := bid.Pack64(false, c.coeff, c.exp)
		if !ok || bid.IsSubnormal64(x) != c.want {
			t.Errorf("IsSubnormal64(%de%d) = %t, want %t", c.coeff, c.exp, bid.IsSubnormal64(x), c.want)
		}
	}
	nines33 := toUint128(new(big.Int).Sub(new(big.Int).Exp(big.NewInt(10), big.NewInt(33), nil), big.NewInt(1)))
	ten33 := toUint128(new(big.Int).Exp(big.NewInt(10), big.NewInt(33), nil))
	max34 := bid.Uint128{Hi: 0x1ED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF}
	sub128 := []struct {
		coeff bid.Uint128
		exp   int
		want  bool
	}{
		{bid.Uint128{Lo: 1}, -6143, false},
		{bid.Uint128{Lo: 1}, -6144, true},
		{bid.Uint128{Lo: 1}, -6176, true},
		{nines33, -6176, true},
		{ten33, -6176, false},
		{max34, -6176, false},
		{bid.Uint128{}, -6176, false},
	}
	for _, c := range sub128 {
		x, ok := bid.Pack128(false, c.coeff, c.exp)
		if !ok || bid.IsSubnormal128(x) != c.want {
			t.Errorf("IsSubnormal128(%s e%d) = %t, want %t", hex128(c.coeff), c.exp, bid.IsSubnormal128(x), c.want)
		}
	}
}

var packCoeffs64 = []uint64{
	0, 1, 9, 10, 99, 1<<53 - 2, 1<<53 - 1, // the first form
	1 << 53, 1<<53 + 1, 1<<53 + 1<<49, // the second form; G12 is never set below 10^16
	999999999999999, 1000000000000000, ieeeMax64 - 1, ieeeMax64,
}

var packCoeffs128 = []bid.Uint128{
	{}, {Lo: 1}, {Lo: 9}, {Lo: 10}, {Lo: 1<<64 - 1}, {Hi: 1}, {Hi: 1, Lo: 1},
	{Hi: 0x1ED09BEAD87C0, Lo: 0x378D8E63FFFFFFFE},
	{Hi: 0x1ED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF}, // 10^34-1
}

// Pack64 emits exactly the canonical form of the standard for every
// exponent and every boundary coefficient, in both signs, and nothing else.
func TestPack64Canonical(t *testing.T) {
	for _, neg := range []bool{false, true} {
		for exp := -398; exp <= 369; exp++ {
			for _, c := range packCoeffs64 {
				x, ok := bid.Pack64(neg, c, exp)
				if !ok {
					t.Fatalf("Pack64(%t, %d, %d) refused", neg, c, exp)
				}
				if want := ieeeEncode64(neg, c, exp); x != want {
					t.Fatalf("Pack64(%t, %d, %d) = %016x, want %016x", neg, c, exp, x, want)
				}
				if !bid.IsCanonical64(x) || !bid.IsFinite64(x) {
					t.Fatalf("Pack64(%t, %d, %d) = %016x is not a canonical finite encoding", neg, c, exp, x)
				}
				form2 := x>>61&3 == 3
				if form2 != (c >= 1<<53) {
					t.Fatalf("Pack64(%t, %d, %d) = %016x: wrong form", neg, c, exp, x)
				}
				kind, n, c2, e2 := bid.Unpack64(x)
				if kind != bid.KindFinite || n != neg || c2 != c || e2 != exp {
					t.Fatalf("Unpack64(Pack64(%t, %d, %d)) = (%d, %t, %d, %d)", neg, c, exp, kind, n, c2, e2)
				}
			}
		}
	}
	// Hand-checked literals.
	for _, c := range []struct {
		neg   bool
		coeff uint64
		exp   int
		want  uint64
	}{
		{false, 0, 0, 0x31C0000000000000},
		{true, 0, 0, 0xB1C0000000000000},
		{false, 1, 0, 0x31C0000000000001},
		{false, 1, -398, 1},
		{false, 1<<53 - 1, 0, 0x31C0000000000000 | (1<<53 - 1)},
		{false, 1 << 53, 0, 0x6C70000000000000},     // 11 | 398<<51 | 0
		{false, ieeeMax64, 369, 0x77FB86F26FC0FFFF}, // 3<<61 | 767<<51 | 992800745259007
		{true, ieeeMax64, 369, 0xF7FB86F26FC0FFFF},
	} {
		if x, ok := bid.Pack64(c.neg, c.coeff, c.exp); !ok || x != c.want {
			t.Errorf("Pack64(%t, %d, %d) = %016x, %t, want %016x", c.neg, c.coeff, c.exp, x, ok, c.want)
		}
	}
}

func TestPack64Refusals(t *testing.T) {
	for _, neg := range []bool{false, true} {
		for _, c := range []struct {
			coeff uint64
			exp   int
		}{
			{ieeeMax64 + 1, 0}, {1<<53 + 1<<51 - 1, 0}, {1<<53 + 1<<51, 0}, {math.MaxUint64, 0},
			{0, -399}, {0, 370}, {1, math.MinInt}, {1, math.MaxInt}, {ieeeMax64 + 1, -399},
		} {
			x, ok := bid.Pack64(neg, c.coeff, c.exp)
			if ok || x != bid.NaN64 {
				t.Errorf("Pack64(%t, %d, %d) = %016x, %t, want NaN64, false", neg, c.coeff, c.exp, x, ok)
			}
		}
	}
}

func TestPack128Canonical(t *testing.T) {
	for _, neg := range []bool{false, true} {
		for exp := -6176; exp <= 6111; exp++ {
			for _, c := range packCoeffs128 {
				x, ok := bid.Pack128(neg, c, exp)
				if !ok {
					t.Fatalf("Pack128(%t, %s, %d) refused", neg, hex128(c), exp)
				}
				if want := ieeeEncode128(neg, c, exp); x != want {
					t.Fatalf("Pack128(%t, %s, %d) = %s, want %s", neg, hex128(c), exp, hex128(x), hex128(want))
				}
				if !bid.IsCanonical128(x) || !bid.IsFinite128(x) {
					t.Fatalf("Pack128(%t, %s, %d) = %s is not a canonical finite encoding", neg, hex128(c), exp, hex128(x))
				}
				if x.Hi>>61&3 == 3 {
					t.Fatalf("Pack128(%t, %s, %d) = %s uses the second form", neg, hex128(c), exp, hex128(x))
				}
				kind, n, c2, e2 := bid.Unpack128(x)
				if kind != bid.KindFinite || n != neg || c2 != c || e2 != exp {
					t.Fatalf("Unpack128(Pack128(%t, %s, %d)) = (%d, %t, %s, %d)", neg, hex128(c), exp, kind, n, hex128(c2), e2)
				}
			}
		}
	}
	for _, c := range []struct {
		neg   bool
		coeff bid.Uint128
		exp   int
		want  bid.Uint128
	}{
		{false, bid.Uint128{}, 0, bid.Uint128{Hi: 0x3040000000000000}},
		{true, bid.Uint128{}, 0, bid.Uint128{Hi: 0xB040000000000000}},
		{false, bid.Uint128{Lo: 1}, -6176, bid.Uint128{Lo: 1}},
		{false, bid.Uint128{Hi: 0x1ED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF}, 6111, bid.Uint128{Hi: 0x5FFFED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF}},
		{true, bid.Uint128{Hi: 0x1ED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF}, 6111, bid.Uint128{Hi: 0xDFFFED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF}},
	} {
		if x, ok := bid.Pack128(c.neg, c.coeff, c.exp); !ok || x != c.want {
			t.Errorf("Pack128(%t, %s, %d) = %s, %t, want %s", c.neg, hex128(c.coeff), c.exp, hex128(x), ok, hex128(c.want))
		}
	}
}

func TestPack128Refusals(t *testing.T) {
	over := []bid.Uint128{
		{Hi: 0x1ED09BEAD87C0, Lo: 0x378D8E6400000000}, // 10^34
		{Hi: 1<<49 - 1, Lo: math.MaxUint64},           // 2^113-1
		{Hi: 1 << 49},                                 // 2^113
		{Hi: math.MaxUint64, Lo: math.MaxUint64},
	}
	for _, neg := range []bool{false, true} {
		for _, c := range over {
			if x, ok := bid.Pack128(neg, c, 0); ok || x != (bid.Uint128{Hi: bid.NaN128Hi}) {
				t.Errorf("Pack128(%t, %s, 0) = %s, %t, want NaN128, false", neg, hex128(c), hex128(x), ok)
			}
		}
		for _, exp := range []int{-6177, 6112, math.MinInt, math.MaxInt} {
			if x, ok := bid.Pack128(neg, bid.Uint128{Lo: 1}, exp); ok || x != (bid.Uint128{Hi: bid.NaN128Hi}) {
				t.Errorf("Pack128(%t, 1, %d) = %s, %t, want NaN128, false", neg, exp, hex128(x), ok)
			}
		}
	}
}

// A decoded non-canonical encoding re-encodes as the canonical zero with
// its own sign and exponent; the exponent is kept even for a second-form
// decimal128, as the contract requires.
func TestNonCanonicalZeroKeepsSignAndExponent(t *testing.T) {
	// decimal64 second form, coefficient 10^16, exponent 5, negative.
	x := uint64(1<<63 | 3<<61 | (5+398)<<51 | 992800745259008)
	kind, neg, c, exp := bid.Unpack64(x)
	if kind != bid.KindFinite || !neg || c != 0 || exp != 5 || !bid.IsZero64(x) || bid.IsCanonical64(x) {
		t.Errorf("Unpack64(%016x) = (%d, %t, %d, %d)", x, kind, neg, c, exp)
	}
	if y, ok := bid.Pack64(neg, c, exp); !ok || y != 1<<63|(5+398)<<53 {
		t.Errorf("Pack64 of the decoded zero = %016x, %t", y, ok)
	}
	// decimal128 second form, exponent -7, positive.
	z := bid.Uint128{Hi: 3<<61 | (6176-7)<<47 | 12345, Lo: 6789}
	kind, neg, cz, exp := bid.Unpack128(z)
	if kind != bid.KindFinite || neg || cz != (bid.Uint128{}) || exp != -7 || !bid.IsZero128(z) || bid.IsCanonical128(z) {
		t.Errorf("Unpack128(%s) = (%d, %t, %s, %d)", hex128(z), kind, neg, hex128(cz), exp)
	}
	if y, ok := bid.Pack128(neg, cz, exp); !ok || y != (bid.Uint128{Hi: (6176 - 7) << 49}) {
		t.Errorf("Pack128 of the decoded zero = %s, %t", hex128(y), ok)
	}
}

func TestSignOperationsOnEncodings(t *testing.T) {
	for _, x := range []uint64{0, 1, 0x7C00000000000001, 0x7E000000DEADBEEF, 0xF800000000000000, 3<<61 | 992800745259008} {
		if bid.Neg64(x) != x^1<<63 || bid.Abs64(x) != x&^(1<<63) || bid.CopySign64(x, 1<<63) != x|1<<63 || bid.CopySign64(x, 0) != x&^(1<<63) {
			t.Errorf("sign operations on %016x", x)
		}
	}
	for _, x := range []bid.Uint128{{}, {Lo: 5}, {Hi: 0x7E000000DEADBEEF, Lo: 7}, {Hi: 3 << 61, Lo: 9}} {
		neg := bid.Uint128{Hi: x.Hi ^ 1<<63, Lo: x.Lo}
		abs := bid.Uint128{Hi: x.Hi &^ (1 << 63), Lo: x.Lo}
		if bid.Neg128(x) != neg || bid.Abs128(x) != abs || bid.CopySign128(x, bid.Uint128{Hi: 1 << 63}) != (bid.Uint128{Hi: x.Hi | 1<<63, Lo: x.Lo}) ||
			bid.CopySign128(x, bid.Uint128{Lo: 1}) != abs {
			t.Errorf("sign operations on %s", hex128(x))
		}
	}
}

// FuzzUnpack64: decoding never panics, matches the IEEE decoder, and
// canonical encodings survive a decode-encode round trip. The seeds run
// under go test -short.
func FuzzUnpack64(f *testing.F) {
	for _, sign := range []uint64{0, 1 << 63} {
		for _, g := range []uint64{0, 1, 0x7FF, 0x800, 0x17FF, 0x1000, 0x17FF, 0x1800, 0x1BFF, 0x1C00, 0x1DFF, 0x1E00, 0x1EFF, 0x1F00, 0x1F7F, 0x1F80, 0x1FFF} {
			for _, tr := range trailing64 {
				f.Add(sign | g<<50 | tr)
			}
		}
	}
	f.Fuzz(func(t *testing.T, x uint64) { checkDecode64(t, x) })
}

func FuzzUnpack128(f *testing.F) {
	for _, sign := range []uint64{0, 1 << 63} {
		for _, g := range []uint64{0, 1, 0x7FFF, 0x8000, 0xBFFF, 0xC000, 0xC7FF, 0xCFFF, 0xD000, 0xF000, 0xF7FF, 0xF800, 0xFFFF, 0x1FFFF, 0x1F000, 0x1F800, 0x1E000, 0x1E800} {
			for _, tr := range trailing128 {
				f.Add(sign|g<<46|tr.Hi, tr.Lo)
			}
		}
	}
	f.Fuzz(func(t *testing.T, hi, lo uint64) { checkDecode128(t, bid.Uint128{Hi: hi, Lo: lo}) })
}

// FuzzPack64 checks that encode-then-decode is the identity on every
// canonical input, and that Pack64 refuses exactly the others.
func FuzzPack64(f *testing.F) {
	for _, c := range packCoeffs64 {
		for _, e := range []int{-398, -397, -1, 0, 1, 368, 369} {
			f.Add(false, c, e)
			f.Add(true, c, e)
		}
	}
	f.Add(false, uint64(ieeeMax64+1), 0)
	f.Add(true, uint64(math.MaxUint64), 0)
	f.Add(false, uint64(1), -399)
	f.Add(false, uint64(1), 370)
	f.Add(false, uint64(1), math.MinInt)
	f.Add(false, uint64(1), math.MaxInt)
	f.Fuzz(func(t *testing.T, neg bool, coeff uint64, exp int) {
		x, ok := bid.Pack64(neg, coeff, exp)
		if inRange := coeff <= ieeeMax64 && exp >= -398 && exp <= 369; ok != inRange {
			t.Fatalf("Pack64(%t, %d, %d) ok = %t", neg, coeff, exp, ok)
		}
		if !ok {
			if x != bid.NaN64 {
				t.Fatalf("Pack64(%t, %d, %d) = %016x on refusal", neg, coeff, exp, x)
			}
			return
		}
		if x != ieeeEncode64(neg, coeff, exp) || !bid.IsCanonical64(x) {
			t.Fatalf("Pack64(%t, %d, %d) = %016x", neg, coeff, exp, x)
		}
		kind, n, c, e := bid.Unpack64(x)
		if kind != bid.KindFinite || n != neg || c != coeff || e != exp {
			t.Fatalf("Unpack64(Pack64(%t, %d, %d)) = (%d, %t, %d, %d)", neg, coeff, exp, kind, n, c, e)
		}
		checkDecode64(t, x)
	})
}

func FuzzPack128(f *testing.F) {
	for _, c := range packCoeffs128 {
		for _, e := range []int{-6176, -6175, -1, 0, 1, 6110, 6111} {
			f.Add(false, c.Hi, c.Lo, e)
			f.Add(true, c.Hi, c.Lo, e)
		}
	}
	f.Add(false, uint64(0x1ED09BEAD87C0), uint64(0x378D8E6400000000), 0) // 10^34
	f.Add(true, uint64(math.MaxUint64), uint64(math.MaxUint64), 0)
	f.Add(false, uint64(0), uint64(1), -6177)
	f.Add(false, uint64(0), uint64(1), 6112)
	f.Add(false, uint64(0), uint64(1), math.MinInt)
	f.Add(false, uint64(0), uint64(1), math.MaxInt)
	f.Fuzz(func(t *testing.T, neg bool, hi, lo uint64, exp int) {
		coeff := bid.Uint128{Hi: hi, Lo: lo}
		x, ok := bid.Pack128(neg, coeff, exp)
		inCoeff := hi < ieeeMax128H || hi == ieeeMax128H && lo <= ieeeMax128L
		if inRange := inCoeff && exp >= -6176 && exp <= 6111; ok != inRange {
			t.Fatalf("Pack128(%t, %s, %d) ok = %t", neg, hex128(coeff), exp, ok)
		}
		if !ok {
			if x != (bid.Uint128{Hi: bid.NaN128Hi}) {
				t.Fatalf("Pack128(%t, %s, %d) = %s on refusal", neg, hex128(coeff), exp, hex128(x))
			}
			return
		}
		if x != ieeeEncode128(neg, coeff, exp) || !bid.IsCanonical128(x) {
			t.Fatalf("Pack128(%t, %s, %d) = %s", neg, hex128(coeff), exp, hex128(x))
		}
		kind, n, c, e := bid.Unpack128(x)
		if kind != bid.KindFinite || n != neg || c != coeff || e != exp {
			t.Fatalf("Unpack128(Pack128(%t, %s, %d)) = (%d, %t, %s, %d)", neg, hex128(coeff), exp, kind, n, hex128(c), e)
		}
		checkDecode128(t, x)
	})
}
