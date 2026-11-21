// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go sign.go.

package d128

func signBitOf(signFlag bool) int {
	if signFlag {
		return 1
	}
	return 0
}

// boolToInt64 is the int64 spelling of signBitOf, used when packing a sign into
// a dword field.
func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
