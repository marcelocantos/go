// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go u128.go.

package d128

type u128 struct {
	dw0 int64
	dw1 int64
}

// u128FromU64 builds a u128 from unsigned low/high halves (primitive-interior
// spelling, §3.4).
func u128FromU64(dw0, dw1 uint64) u128 {
	return u128{dw0: int64(dw0), dw1: int64(dw1)}
}
