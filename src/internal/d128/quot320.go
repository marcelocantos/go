// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go quot320.go.

package d128

// quot320Residue is the D48 divPow10 result: a 320-bit quotient with the
// residue of the discarded digits (divPow10_q320res_*; Swift Quot320.swift).
type quot320Residue struct {
	quotDw0 int64
	quotDw1 int64
	quotDw2 int64
	quotDw3 int64
	quotDw4 int64
	residue residue
}

func (r quot320Residue) quot() u320 {
	return u320{r.quotDw0, r.quotDw1, r.quotDw2, r.quotDw3, r.quotDw4}
}

// quot320Rem160 is the divRem_320_160_mg result: a 320-bit quotient and a
// remainder below the (<= 160-bit) divisor.
type quot320Rem160 struct {
	quotDw0 int64
	quotDw1 int64
	quotDw2 int64
	quotDw3 int64
	quotDw4 int64
	remDw0  int64
	remDw1  int64
	remDw2  int64
}

func (r quot320Rem160) quot() u320 {
	return u320{r.quotDw0, r.quotDw1, r.quotDw2, r.quotDw3, r.quotDw4}
}

func (r quot320Rem160) rem() u192 { return u192{r.remDw0, r.remDw1, r.remDw2} }
