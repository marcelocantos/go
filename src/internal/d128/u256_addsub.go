// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go u256_addsub.go.

package d128

// subAbs_d128swap_128x128 returns `|x - y|` and whether the operands were swapped (y > x).
// Branch-free conditional negate; the borrow flag is the swap flag (D14).
func subAbs_d128swap_128x128(xDw0, xDw1, yDw0, yDw1 int64) diff128Swap {
	diff, borrow := sub128Borrow(u128{xDw0, xDw1}, u128{yDw0, yDw1})
	swap := borrow != 0
	if swap {
		diff, _ = sub128Borrow(u128{0, 0}, diff) // negate: |x-y| = 0 - (x-y)
	}
	return diff128Swap{diff.dw0, diff.dw1, swap}
}

// cmp_int_256x256 compares two 256-bit values as unsigned magnitudes, returning
// -1, 0, or 1. Sibling of cmp_int_128x128, one tier up.
func cmp_int_256x256(xDw0, xDw1, xDw2, xDw3, yDw0, yDw1, yDw2, yDw3 int64) int {
	if uint64(xDw3) != uint64(yDw3) {
		if uint64(xDw3) < uint64(yDw3) {
			return -1
		}
		return 1
	}
	if uint64(xDw2) != uint64(yDw2) {
		if uint64(xDw2) < uint64(yDw2) {
			return -1
		}
		return 1
	}
	if uint64(xDw1) != uint64(yDw1) {
		if uint64(xDw1) < uint64(yDw1) {
			return -1
		}
		return 1
	}
	if uint64(xDw0) != uint64(yDw0) {
		if uint64(xDw0) < uint64(yDw0) {
			return -1
		}
		return 1
	}
	return 0
}
