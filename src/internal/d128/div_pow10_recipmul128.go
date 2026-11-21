// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go div_pow10_recipmul128.go.

package d128

// const_RMP10_128_M[pow10] (lo/hi dwords) is the 128-bit reciprocal multiplier; index 0
// is unused (pow10 is 1-based).
var const_RMP10_128_M = [78]int64{
	0, 0, -3689348814741910323, -3689348814741910324, -8116567392432202711, 2951479051793528258,
	-5607810198407703691, 4722366482869645213, -796899343984252629, 3777893186295716170, 7059937891890119595, 1511157274518286468,
	-5966710051105796727, 302231454903657293, 8681362423783412279, 120892581961462917, -433607690457090823, 96714065569170333,
	3342462662376237665, 77371252455336267, -2352363749791415257, 30948500982134506, 6908224879525537595, 6189700196426901,
	1837231088878519753, 4951760157141521, -2219563943639094521, 3961408125713216, -4577174392197548131, 1584563250285286,
	7406306930467692465, 1267650600228229, -1104043714324185915, 126765060022822, -5455818757660607787, 202824096036516,
	6287533877951699089, 40564819207303, -5705333293747935177, 129807421463370, 4930221287926428315, 3245185536584,
	-6359384767086891331, 10384593717069, -5087507813669513065, 8307674973655, 5343694504016015421, 3323069989462,
	-1551871013135504155, 1329227995784, -310374202627100831, 265845599156, -7626996991585501311, 212676479325,
	2163949416424810061, 42535295865, 1731159533139848049, 34028236692, -5993770002971942207, 27222589353,
	-2211334375271286885, 43556142965, 6494163879375305893, 17422457186, -6729281241546290057, 1742245718,
	-3388152356990243445, 2787593149, -677630471398048689, 557518629, -542104377118438951, 446014903,
	6511330626094318325, 713623846, -8463514193788003639, 285449538, -5382051653499511051, 57089907,
}

// const_RMP10_128_S128[pow10] = S - 128 (the post-multiply shift in the high half).
var const_RMP10_128_S128 = [39]int{
	0, 2, 2, 5, 7, 8, 8, 9, 11, 13, 14, 14, 16, 18, 19, 21, 20, 23, 23, 27, 24, 28, 30, 31, 32,
	32, 34, 34, 36, 38, 41, 42, 41, 44, 44, 46, 49, 50, 50,
}

// Dword accessors over the RMP10_128_M table; dw0/dw1 are the low/high 64 bits
// of the 5^k reciprocal M. Mirrors rmp10_256_dw*.
func rmp10_128_dw0(k int) int64 { return const_RMP10_128_M[k<<1] }

func rmp10_128_dw1(k int) int64 { return const_RMP10_128_M[(k<<1)|1] }

// Single reciprocal-multiply kernel for x / 10^pow10 (128-bit dividend), pow10
// 1..38, returning the quotient and the classified residue. One 128*128->256
// multiply plus a shift, uniform across the whole range. Backs Core's divPow10
// dispatch for pow10 >= 2 (pow10 == 1 keeps the divide-by-10 magic).
func divPow10_q128res_128_recipmul(xDw0, xDw1 int64, pow10 int) quot128Residue {
	demand("recipmul: pow10 outside [1, 38]", pow10 > 0 && pow10 <= 38)
	k := pow10
	mDw0 := rmp10_128_dw0(pow10)
	mDw1 := rmp10_128_dw1(pow10)
	s128 := const_RMP10_128_S128[pow10]

	// Strip k-1 low bits (the 2^(k-1) factor of 10^k); track them as sticky.
	dividendAlfa := shr_128_128(xDw0, xDw1, k-1)
	stickyAlfa := false
	if k-1 > 0 {
		one := shl_128_128(1, 0, k-1)
		alfaMask := sub_128_128x128(one.dw0, one.dw1, 1, 0)
		masked := and_128_128x128(xDw0, xDw1, alfaMask.dw0, alfaMask.dw1)
		stickyAlfa = !isZero_bool_128(masked.dw0, masked.dw1)
	}

	// Multiply the stripped dividend by the 5^k reciprocal -> 256-bit product.
	p := mul_256_128x128(dividendAlfa.dw0, dividendAlfa.dw1, mDw0, mDw1)
	pHi := p.hi()
	pLo := p.lo()

	// quotPlusRound = prod >> S = pHi >> s128.
	quotPlusRound := shr_128_128(pHi.dw0, pHi.dw1, s128)
	q := shr_128_128(quotPlusRound.dw0, quotPlusRound.dw1, 1)
	roundBit := (quotPlusRound.dw0 & 1) != 0

	// stickyBeta = (prod mod 2^S != 0) || (pLo >= M), no 256-bit compare.
	stickyBeta := false
	if s128 > 0 {
		one := shl_128_128(1, 0, s128)
		fracMask := sub_128_128x128(one.dw0, one.dw1, 1, 0)
		highFrac := and_128_128x128(pHi.dw0, pHi.dw1, fracMask.dw0, fracMask.dw1)
		stickyBeta = !isZero_bool_128(highFrac.dw0, highFrac.dw1)
	}
	if !stickyBeta && cmp_int_128x128(pLo.dw0, pLo.dw1, mDw0, mDw1) >= 0 {
		stickyBeta = true
	}

	sticky := stickyAlfa || stickyBeta

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
