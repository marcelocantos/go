// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go u320.go.

package d128

type u192 struct {
	dw0 int64
	dw1 int64
	dw2 int64
}

type u320 struct {
	dw0 int64
	dw1 int64
	dw2 int64
	dw3 int64
	dw4 int64
}

// u192 narrows to a u192; the value must fit (dw3 == dw4 == 0).
func (x u320) u192() u192 {

	return u192{x.dw0, x.dw1, x.dw2}
}
