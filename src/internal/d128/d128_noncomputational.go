// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go d128_noncomputational.go.

package d128

func d128_negate(x d128) d128 {
	return d128{x.ubdLo64, x.ubdHi64 ^ (int64(1) << 49)}
}
