// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package staticdata

import (
	"encoding/base64"
	"fmt"
	"go/constant"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"cmd/compile/internal/base"
	"cmd/compile/internal/ir"
	"cmd/compile/internal/objw"
	"cmd/compile/internal/types"
	"cmd/internal/hash"
	"cmd/internal/obj"
	"cmd/internal/objabi"
	"cmd/internal/src"
)

// InitAddrOffset writes the static name symbol lsym to n, it does not modify n.
// It's the caller responsibility to make sure lsym is from ONAME/PEXTERN node.
func InitAddrOffset(n *ir.Name, noff int64, lsym *obj.LSym, off int64) {
	if n.Op() != ir.ONAME {
		base.Fatalf("InitAddr n op %v", n.Op())
	}
	if n.Sym() == nil {
		base.Fatalf("InitAddr nil n sym")
	}
	s := n.Linksym()
	s.WriteAddr(base.Ctxt, noff, types.PtrSize, lsym, off)
}

// InitAddr is InitAddrOffset, with offset fixed to 0.
func InitAddr(n *ir.Name, noff int64, lsym *obj.LSym) {
	InitAddrOffset(n, noff, lsym, 0)
}

// InitSlice writes a static slice symbol {lsym, lencap, lencap} to n+noff, it does not modify n.
// It's the caller responsibility to make sure lsym is from ONAME node.
func InitSlice(n *ir.Name, noff int64, lsym *obj.LSym, lencap int64) {
	s := n.Linksym()
	s.WriteAddr(base.Ctxt, noff, types.PtrSize, lsym, 0)
	s.WriteInt(base.Ctxt, noff+types.SliceLenOffset, types.PtrSize, lencap)
	s.WriteInt(base.Ctxt, noff+types.SliceCapOffset, types.PtrSize, lencap)
}

func InitSliceBytes(nam *ir.Name, off int64, s string) {
	if nam.Op() != ir.ONAME {
		base.Fatalf("InitSliceBytes %v", nam)
	}
	InitSlice(nam, off, slicedata(nam.Pos(), s), int64(len(s)))
}

const (
	stringSymPrefix  = "go:string."
	stringSymPattern = ".gostring.%d.%s"
)

// shortHashString converts the hash to a string for use with stringSymPattern.
// We cut it to 16 bytes and then base64-encode to make it even smaller.
func shortHashString(hash []byte) string {
	return base64.StdEncoding.EncodeToString(hash[:16])
}

// StringSym returns a symbol containing the string s.
// The symbol contains the string data, not a string header.
func StringSym(pos src.XPos, s string) (data *obj.LSym) {
	var symname string
	if len(s) > 100 {
		// Huge strings are hashed to avoid long names in object files.
		// Indulge in some paranoia by writing the length of s, too,
		// as protection against length extension attacks.
		// Same pattern is known to fileStringSym below.
		h := hash.New32()
		io.WriteString(h, s)
		symname = fmt.Sprintf(stringSymPattern, len(s), shortHashString(h.Sum(nil)))
	} else {
		// Small strings get named directly by their contents.
		symname = strconv.Quote(s)
	}

	symdata := base.Ctxt.Lookup(stringSymPrefix + symname)
	if !symdata.OnList() {
		off := dstringdata(symdata, 0, s, pos, "string")
		objw.Global(symdata, int32(off), obj.DUPOK|obj.RODATA|obj.LOCAL)
		symdata.Set(obj.AttrContentAddressable, true)
	}

	return symdata
}

// StringSymNoCommon is like StringSym, but produces a symbol that is not content-
// addressable. This symbol is not supposed to appear in the final binary, it is
// only used to pass string arguments to the linker like R_USENAMEDMETHOD does.
func StringSymNoCommon(s string) (data *obj.LSym) {
	var nameSym obj.LSym
	nameSym.WriteString(base.Ctxt, 0, len(s), s)
	objw.Global(&nameSym, int32(len(s)), obj.RODATA)
	return &nameSym
}

// maxFileSize is the maximum file size permitted by the linker
// (see issue #9862).
const maxFileSize = int64(2e9)

// fileStringSym returns a symbol for the contents and the size of file.
// If readonly is true, the symbol shares storage with any literal string
// or other file with the same content and is placed in a read-only section.
// If readonly is false, the symbol is a read-write copy separate from any other,
// for use as the backing store of a []byte.
// The content hash of file is copied into hashBytes. (If hash is nil, nothing is copied.)
// The returned symbol contains the data itself, not a string header.
func fileStringSym(pos src.XPos, file string, readonly bool, hashBytes []byte) (*obj.LSym, int64, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("not a regular file")
	}
	size := info.Size()
	if size <= 1*1024 {
		data, err := io.ReadAll(f)
		if err != nil {
			return nil, 0, err
		}
		if int64(len(data)) != size {
			return nil, 0, fmt.Errorf("file changed between reads")
		}
		var sym *obj.LSym
		if readonly {
			sym = StringSym(pos, string(data))
		} else {
			sym = slicedata(pos, string(data))
		}
		if len(hashBytes) > 0 {
			sum := hash.Sum32(data)
			copy(hashBytes, sum[:])
		}
		return sym, size, nil
	}
	if size > maxFileSize {
		// ggloblsym takes an int32,
		// and probably the rest of the toolchain
		// can't handle such big symbols either.
		// See golang.org/issue/9862.
		return nil, 0, fmt.Errorf("file too large (%d bytes > %d bytes)", size, maxFileSize)
	}

	// File is too big to read and keep in memory.
	// Compute hashBytes if needed for read-only content hashing or if the caller wants it.
	var sum []byte
	if readonly || len(hashBytes) > 0 {
		h := hash.New32()
		n, err := io.Copy(h, f)
		if err != nil {
			return nil, 0, err
		}
		if n != size {
			return nil, 0, fmt.Errorf("file changed between reads")
		}
		sum = h.Sum(nil)
		copy(hashBytes, sum)
	}

	var symdata *obj.LSym
	if readonly {
		symname := fmt.Sprintf(stringSymPattern, size, shortHashString(sum))
		symdata = base.Ctxt.Lookup(stringSymPrefix + symname)
		if !symdata.OnList() {
			info := symdata.NewFileInfo()
			info.Name = file
			info.Size = size
			objw.Global(symdata, int32(size), obj.DUPOK|obj.RODATA|obj.LOCAL)
			// Note: AttrContentAddressable cannot be set here,
			// because the content-addressable-handling code
			// does not know about file symbols.
		}
	} else {
		// Emit a zero-length data symbol
		// and then fix up length and content to use file.
		symdata = slicedata(pos, "")
		symdata.Size = size
		symdata.Type = objabi.SNOPTRDATA
		info := symdata.NewFileInfo()
		info.Name = file
		info.Size = size
	}

	return symdata, size, nil
}

var slicedataGen int

func slicedata(pos src.XPos, s string) *obj.LSym {
	slicedataGen++
	symname := fmt.Sprintf(".gobytes.%d", slicedataGen)
	lsym := types.LocalPkg.Lookup(symname).LinksymABI(obj.ABI0)
	off := dstringdata(lsym, 0, s, pos, "slice")
	objw.Global(lsym, int32(off), obj.NOPTR|obj.LOCAL)

	return lsym
}

func dstringdata(s *obj.LSym, off int, t string, pos src.XPos, what string) int {
	// Objects that are too large will cause the data section to overflow right away,
	// causing a cryptic error message by the linker. Check for oversize objects here
	// and provide a useful error message instead.
	if int64(len(t)) > 2e9 {
		base.ErrorfAt(pos, 0, "%v with length %v is too big", what, len(t))
		return 0
	}

	s.WriteString(base.Ctxt, int64(off), len(t), t)
	return off + len(t)
}

var (
	funcsymsmu sync.Mutex // protects funcsyms and associated package lookups (see func funcsym)
	funcsyms   []*ir.Name // functions that need function value symbols
)

// FuncLinksym returns n·f, the function value symbol for n.
func FuncLinksym(n *ir.Name) *obj.LSym {
	if n.Op() != ir.ONAME || n.Class != ir.PFUNC {
		base.Fatalf("expected func name: %v", n)
	}
	s := n.Sym()

	// funcsymsmu here serves to protect not just mutations of funcsyms (below),
	// but also the package lookup of the func sym name,
	// since this function gets called concurrently from the backend.
	// There are no other concurrent package lookups in the backend,
	// except for the types package, which is protected separately.
	// Reusing funcsymsmu to also cover this package lookup
	// avoids a general, broader, expensive package lookup mutex.
	funcsymsmu.Lock()
	sf, existed := s.Pkg.LookupOK(ir.FuncSymName(s))
	if !existed {
		funcsyms = append(funcsyms, n)
	}
	funcsymsmu.Unlock()

	return sf.Linksym()
}

func GlobalLinksym(n *ir.Name) *obj.LSym {
	if n.Op() != ir.ONAME || n.Class != ir.PEXTERN {
		base.Fatalf("expected global variable: %v", n)
	}
	return n.Linksym()
}

func WriteFuncSyms() {
	slices.SortFunc(funcsyms, func(a, b *ir.Name) int {
		return strings.Compare(a.Linksym().Name, b.Linksym().Name)
	})
	for _, nam := range funcsyms {
		s := nam.Sym()
		sf := s.Pkg.Lookup(ir.FuncSymName(s)).Linksym()

		// While compiling package runtime, we might try to create
		// funcsyms for functions from both types.LocalPkg and
		// ir.Pkgs.Runtime.
		if base.Flag.CompilingRuntime && sf.OnList() {
			continue
		}

		// Function values must always reference ABIInternal
		// entry points.
		target := s.Linksym()
		if target.ABI() != obj.ABIInternal {
			base.Fatalf("expected ABIInternal: %v has %v", target, target.ABI())
		}
		objw.SymPtr(sf, 0, target, 0)
		objw.Global(sf, int32(types.PtrSize), obj.DUPOK|obj.RODATA)
	}
}

// InitConst writes the static literal c to n.
// Neither n nor c is modified.
func InitConst(n *ir.Name, noff int64, c ir.Node, wid int) {
	if n.Op() != ir.ONAME {
		base.Fatalf("InitConst n op %v", n.Op())
	}
	if n.Sym() == nil {
		base.Fatalf("InitConst nil n sym")
	}
	if c.Op() == ir.ONIL {
		return
	}
	if c.Op() != ir.OLITERAL {
		base.Fatalf("InitConst c op %v", c.Op())
	}
	s := n.Linksym()
	switch u := c.Val(); u.Kind() {
	case constant.Bool:
		i := int64(obj.Bool2int(constant.BoolVal(u)))
		s.WriteInt(base.Ctxt, noff, wid, i)

	case constant.Int:
		s.WriteInt(base.Ctxt, noff, wid, ir.IntVal(c.Type(), u))

	case constant.Float:
		f, _ := constant.Float64Val(u)
		switch c.Type().Kind() {
		case types.TFLOAT32:
			s.WriteFloat32(base.Ctxt, noff, float32(f))
		case types.TFLOAT64:
			s.WriteFloat64(base.Ctxt, noff, f)
		case types.TDECIMAL64:
			bid := float64ToBID64(f)
			s.WriteInt(base.Ctxt, noff, 8, int64(bid))
		case types.TDECIMAL128:
			hi, lo := float64ToBID128(f)
			s.WriteInt(base.Ctxt, noff, 8, int64(lo))
			s.WriteInt(base.Ctxt, noff+8, 8, int64(hi))
		}

	case constant.Complex:
		re, _ := constant.Float64Val(constant.Real(u))
		im, _ := constant.Float64Val(constant.Imag(u))
		switch c.Type().Kind() {
		case types.TCOMPLEX64:
			s.WriteFloat32(base.Ctxt, noff, float32(re))
			s.WriteFloat32(base.Ctxt, noff+4, float32(im))
		case types.TCOMPLEX128:
			s.WriteFloat64(base.Ctxt, noff, re)
			s.WriteFloat64(base.Ctxt, noff+8, im)
		}

	case constant.String:
		i := constant.StringVal(u)
		symdata := StringSym(n.Pos(), i)
		s.WriteAddr(base.Ctxt, noff, types.PtrSize, symdata, 0)
		s.WriteInt(base.Ctxt, noff+int64(types.PtrSize), types.PtrSize, int64(len(i)))

	default:
		base.Fatalf("InitConst unhandled OLITERAL %v", c)
	}
}

// float64ToBID64 converts a float64 value to BID64 (IEEE 754-2008 decimal64)
// encoding at compile time for static data initialization.
func float64ToBID64(f float64) uint64 {
	const (
		bid64Bias    = 398
		bid64MaxCoef = uint64(9999999999999999)
		bid64SignBit = uint64(1 << 63)
		bid64Inf     = uint64(0x7800000000000000)
		bid64NaN     = uint64(0x7C00000000000000)
	)

	if f != f {
		return bid64NaN
	}
	var sign uint64
	if f < 0 {
		sign = bid64SignBit
		f = -f
	}
	if f == 0 {
		return sign
	}
	if f > 0 && f+f == f {
		return sign | bid64Inf
	}

	var coeff uint64
	dexp := 0
	fval := f
	for fval >= 10 {
		dexp++
		fval /= 10
	}
	for fval < 1 {
		dexp--
		fval *= 10
	}
	for i := 0; i < 16; i++ {
		digit := uint64(fval)
		coeff = coeff*10 + digit
		fval -= float64(digit)
		fval *= 10
	}
	dexp -= 15
	if uint64(fval) >= 5 {
		coeff++
		if coeff > bid64MaxCoef {
			coeff /= 10
			dexp++
		}
	}
	for coeff > bid64MaxCoef {
		rem := coeff % 10
		coeff /= 10
		dexp++
		if rem > 5 || (rem == 5 && coeff%2 != 0) {
			coeff++
		}
	}
	biasedExp := dexp + bid64Bias
	if biasedExp < 0 || biasedExp > 767 {
		if biasedExp > 767 {
			return sign | bid64Inf
		}
		return sign
	}
	if coeff < (1 << 53) {
		return sign | uint64(biasedExp)<<53 | coeff
	}
	return sign | (3 << 61) | uint64(biasedExp)<<51 | (coeff & ((1 << 51) - 1))
}

// float64ToBID128 converts a float64 value to BID128 (IEEE 754-2008 decimal128)
// encoding at compile time for static data initialization.
// Returns (hi, lo) words.
func float64ToBID128(f float64) (uint64, uint64) {
	const (
		bid128Bias    = 6176
		bid128SignBit = uint64(1 << 63)
		bid128Inf     = uint64(0x7800000000000000)
		bid128NaN     = uint64(0x7C00000000000000)
	)

	if f != f {
		return bid128NaN, 0
	}
	var sign uint64
	if f < 0 {
		sign = bid128SignBit
		f = -f
	}
	if f == 0 {
		return sign, 0
	}
	if f > 0 && f+f == f {
		return sign | bid128Inf, 0
	}

	// Extract 16 significant digits (float64 precision limit).
	// BID128 supports 34 digits but we only have ~16 from float64.
	var coeff uint64
	dexp := 0
	fval := f
	for fval >= 10 {
		dexp++
		fval /= 10
	}
	for fval < 1 {
		dexp--
		fval *= 10
	}
	for i := 0; i < 16; i++ {
		digit := uint64(fval)
		coeff = coeff*10 + digit
		fval -= float64(digit)
		fval *= 10
	}
	dexp -= 15
	if uint64(fval) >= 5 {
		coeff++
	}

	// BID128 Form 1: hi[63]=sign, hi[62:49]=biased_exp, hi[48:0]=coeff_hi, lo=coeff_lo
	// Since coeff fits in 64 bits, coeff_hi = 0 and lo = coeff.
	biasedExp := dexp + bid128Bias
	if biasedExp < 0 || biasedExp > 12287 {
		if biasedExp > 12287 {
			return sign | bid128Inf, 0
		}
		return sign, 0
	}
	hi := sign | uint64(biasedExp)<<49
	return hi, coeff
}
