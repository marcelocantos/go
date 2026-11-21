// Command probe links every prototype so that each is compiled once, as its
// own function, for seg.py to measure.
package main

import (
	"fmt"
	"os"

	"moneyform/ops"
)

func main() {
	a := uint64(len(os.Args))
	var s uint64
	for _, f := range []func(a, b, c, d uint64) (uint64, uint64){
		ops.UAdd, ops.USub, ops.UMul, ops.NAdd, ops.NSub, ops.NMul, ops.CAdd, ops.CSub, ops.CMul,
	} {
		l, h := f(a, a, a, a)
		s += l + h
	}
	for _, f := range []func(a, b, c, d uint64) bool{
		ops.UEq, ops.ULt, ops.NEq, ops.NLt, ops.CEq, ops.CLt,
	} {
		if f(a, a, a, a) {
			s++
		}
	}
	fmt.Println(s, ops.Slow)
}
