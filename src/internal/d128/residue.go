// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go residue.go.

package d128

type residue = int

const (
	const_RESIDUE_EXACT   residue = 0
	const_RESIDUE_LT_HALF residue = 1
	const_RESIDUE_HALF    residue = 2
	const_RESIDUE_GT_HALF residue = 3
)

// const_DIGIT_MAP packs the residue of a single discarded decimal digit two bits
// per digit: 0 → EXACT, 1–4 → LT_HALF, 5 → HALF, 6–9 → GT_HALF.
const const_DIGIT_MAP = 0b11_11_11_11_10_01_01_01_01_00

// Residue of a single discarded decimal digit: 0 -> EXACT, 1-4 -> LT_HALF, 5
// -> HALF, 6-9 -> GT_HALF.
func residueFromDecimalDigit(digit int) residue {

	return (const_DIGIT_MAP >> (digit << 1)) & 3
}
