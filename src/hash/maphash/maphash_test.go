// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package maphash

import (
	"bytes"
	"fmt"
	"hash"
	"internal/asan"
	"internal/bid"
	"internal/testhash"
	"math"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func TestUnseededHash(t *testing.T) {
	m := map[uint64]struct{}{}
	for i := 0; i < 1000; i++ {
		h := new(Hash)
		m[h.Sum64()] = struct{}{}
	}
	if len(m) < 900 {
		t.Errorf("empty hash not sufficiently random: got %d, want 1000", len(m))
	}
}

func TestSeededHash(t *testing.T) {
	s := MakeSeed()
	m := map[uint64]struct{}{}
	for i := 0; i < 1000; i++ {
		h := new(Hash)
		h.SetSeed(s)
		m[h.Sum64()] = struct{}{}
	}
	if len(m) != 1 {
		t.Errorf("seeded hash is random: got %d, want 1", len(m))
	}
}

func TestHashGrouping(t *testing.T) {
	b := bytes.Repeat([]byte("foo"), 100)
	hh := make([]*Hash, 7)
	for i := range hh {
		hh[i] = new(Hash)
	}
	for _, h := range hh[1:] {
		h.SetSeed(hh[0].Seed())
	}
	hh[0].Write(b)
	hh[1].WriteString(string(b))

	writeByte := func(h *Hash, b byte) {
		err := h.WriteByte(b)
		if err != nil {
			t.Fatalf("WriteByte: %v", err)
		}
	}
	writeSingleByte := func(h *Hash, b byte) {
		_, err := h.Write([]byte{b})
		if err != nil {
			t.Fatalf("Write single byte: %v", err)
		}
	}
	writeStringSingleByte := func(h *Hash, b byte) {
		_, err := h.WriteString(string([]byte{b}))
		if err != nil {
			t.Fatalf("WriteString single byte: %v", err)
		}
	}

	for i, x := range b {
		writeByte(hh[2], x)
		writeSingleByte(hh[3], x)
		if i == 0 {
			writeByte(hh[4], x)
		} else {
			writeSingleByte(hh[4], x)
		}
		writeStringSingleByte(hh[5], x)
		if i == 0 {
			writeByte(hh[6], x)
		} else {
			writeStringSingleByte(hh[6], x)
		}
	}

	sum := hh[0].Sum64()
	for i, h := range hh {
		if sum != h.Sum64() {
			t.Errorf("hash %d not identical to a single Write", i)
		}
	}

	if sum1 := Bytes(hh[0].Seed(), b); sum1 != hh[0].Sum64() {
		t.Errorf("hash using Bytes not identical to a single Write")
	}

	if sum1 := String(hh[0].Seed(), string(b)); sum1 != hh[0].Sum64() {
		t.Errorf("hash using String not identical to a single Write")
	}
}

func TestHashBytesVsString(t *testing.T) {
	s := "foo"
	b := []byte(s)
	h1 := new(Hash)
	h2 := new(Hash)
	h2.SetSeed(h1.Seed())
	n1, err1 := h1.WriteString(s)
	if n1 != len(s) || err1 != nil {
		t.Fatalf("WriteString(s) = %d, %v, want %d, nil", n1, err1, len(s))
	}
	n2, err2 := h2.Write(b)
	if n2 != len(b) || err2 != nil {
		t.Fatalf("Write(b) = %d, %v, want %d, nil", n2, err2, len(b))
	}
	if h1.Sum64() != h2.Sum64() {
		t.Errorf("hash of string and bytes not identical")
	}
}

func TestHashHighBytes(t *testing.T) {
	// See issue 34925.
	const N = 10
	m := map[uint64]struct{}{}
	for i := 0; i < N; i++ {
		h := new(Hash)
		h.WriteString("foo")
		m[h.Sum64()>>32] = struct{}{}
	}
	if len(m) < N/2 {
		t.Errorf("from %d seeds, wanted at least %d different hashes; got %d", N, N/2, len(m))
	}
}

func TestRepeat(t *testing.T) {
	h1 := new(Hash)
	h1.WriteString("testing")
	sum1 := h1.Sum64()

	h1.Reset()
	h1.WriteString("testing")
	sum2 := h1.Sum64()

	if sum1 != sum2 {
		t.Errorf("different sum after resetting: %#x != %#x", sum1, sum2)
	}

	h2 := new(Hash)
	h2.SetSeed(h1.Seed())
	h2.WriteString("testing")
	sum3 := h2.Sum64()

	if sum1 != sum3 {
		t.Errorf("different sum on the same seed: %#x != %#x", sum1, sum3)
	}
}

func TestSeedFromSum64(t *testing.T) {
	h1 := new(Hash)
	h1.WriteString("foo")
	x := h1.Sum64() // seed generated here
	h2 := new(Hash)
	h2.SetSeed(h1.Seed())
	h2.WriteString("foo")
	y := h2.Sum64()
	if x != y {
		t.Errorf("hashes don't match: want %x, got %x", x, y)
	}
}

func TestSeedFromSeed(t *testing.T) {
	h1 := new(Hash)
	h1.WriteString("foo")
	_ = h1.Seed() // seed generated here
	x := h1.Sum64()
	h2 := new(Hash)
	h2.SetSeed(h1.Seed())
	h2.WriteString("foo")
	y := h2.Sum64()
	if x != y {
		t.Errorf("hashes don't match: want %x, got %x", x, y)
	}
}

func TestSeedFromFlush(t *testing.T) {
	b := make([]byte, 65)
	h1 := new(Hash)
	h1.Write(b) // seed generated here
	x := h1.Sum64()
	h2 := new(Hash)
	h2.SetSeed(h1.Seed())
	h2.Write(b)
	y := h2.Sum64()
	if x != y {
		t.Errorf("hashes don't match: want %x, got %x", x, y)
	}
}

func TestSeedFromReset(t *testing.T) {
	h1 := new(Hash)
	h1.WriteString("foo")
	h1.Reset() // seed generated here
	h1.WriteString("foo")
	x := h1.Sum64()
	h2 := new(Hash)
	h2.SetSeed(h1.Seed())
	h2.WriteString("foo")
	y := h2.Sum64()
	if x != y {
		t.Errorf("hashes don't match: want %x, got %x", x, y)
	}
}

func negativeZero[T float32 | float64]() T {
	var f T
	f = -f
	return f
}

func TestComparable(t *testing.T) {
	testComparable(t, int64(2))
	testComparable(t, uint64(8))
	testComparable(t, uintptr(12))
	testComparable(t, any("s"))
	testComparable(t, "s")
	testComparable(t, true)
	testComparable(t, new(float64))
	testComparable(t, float64(9))
	testComparable(t, complex128(9i+1))
	testComparable(t, struct{}{})
	testComparable(t, struct {
		i int
		u uint
		b bool
		f float64
		p *int
		a any
	}{i: 9, u: 1, b: true, f: 9.9, p: new(int), a: 1})
	type S struct {
		s string
	}
	s1 := S{s: heapStr(t)}
	s2 := S{s: heapStr(t)}
	if unsafe.StringData(s1.s) == unsafe.StringData(s2.s) {
		t.Fatalf("unexpected two heapStr ptr equal")
	}
	if s1.s != s2.s {
		t.Fatalf("unexpected two heapStr value not equal")
	}
	testComparable(t, s1, s2)
	testComparable(t, s1.s, s2.s)
	c1 := make(chan struct{})
	c2 := make(chan struct{})
	testComparable(t, c1, c1)
	testComparable(t, chan struct{}(nil))
	testComparable(t, float32(0), negativeZero[float32]())
	testComparable(t, float64(0), negativeZero[float64]())
	testComparable(t, decimal64(9))
	testComparable(t, decimal128(42))
	testComparable(t, decimal64(0), math.Decimal64frombits(1<<63)) // +0 == -0
	testComparableNoEqual(t, math.Decimal64NaN(), math.Decimal64NaN())
	testComparableNoEqual(t, math.NaN(), math.NaN())
	testComparableNoEqual(t, [2]string{"a", ""}, [2]string{"", "a"})
	testComparableNoEqual(t, struct{ a, b string }{"foo", ""}, struct{ a, b string }{"", "foo"})
	testComparableNoEqual(t, struct{ a, b any }{int(0), struct{}{}}, struct{ a, b any }{struct{}{}, int(0)})
	testComparableNoEqual(t, c1, c2)
}

func testComparableNoEqual[T comparable](t *testing.T, v1, v2 T) {
	seed := MakeSeed()
	if Comparable(seed, v1) == Comparable(seed, v2) {
		t.Fatalf("Comparable(seed, %v) == Comparable(seed, %v)", v1, v2)
	}
}

var heapStrValue = []byte("aTestString")

func heapStr(t *testing.T) string {
	return string(heapStrValue)
}

func testComparable[T comparable](t *testing.T, v T, v2 ...T) {
	t.Run(reflect.TypeFor[T]().String(), func(t *testing.T) {
		var a, b T = v, v
		if len(v2) != 0 {
			b = v2[0]
		}
		var pa *T = &a
		seed := MakeSeed()
		if Comparable(seed, a) != Comparable(seed, b) {
			t.Fatalf("Comparable(seed, %v) != Comparable(seed, %v)", a, b)
		}
		old := Comparable(seed, pa)
		stackGrow(8192)
		new := Comparable(seed, pa)
		if old != new {
			t.Fatal("Comparable(seed, ptr) != Comparable(seed, ptr)")
		}
	})
}

var use byte

//go:noinline
func stackGrow(dep int) {
	if dep == 0 {
		return
	}
	var local [1024]byte
	// make sure local is allocated on the stack.
	local[randUint64()%1024] = byte(randUint64())
	use = local[randUint64()%1024]
	stackGrow(dep - 1)
}

func TestWriteComparable(t *testing.T) {
	testWriteComparable(t, int64(2))
	testWriteComparable(t, uint64(8))
	testWriteComparable(t, uintptr(12))
	testWriteComparable(t, any("s"))
	testWriteComparable(t, "s")
	testComparable(t, true)
	testWriteComparable(t, new(float64))
	testWriteComparable(t, float64(9))
	testWriteComparable(t, complex128(9i+1))
	testWriteComparable(t, struct{}{})
	testWriteComparable(t, struct {
		i int
		u uint
		b bool
		f float64
		p *int
		a any
	}{i: 9, u: 1, b: true, f: 9.9, p: new(int), a: 1})
	type S struct {
		s string
	}
	s1 := S{s: heapStr(t)}
	s2 := S{s: heapStr(t)}
	if unsafe.StringData(s1.s) == unsafe.StringData(s2.s) {
		t.Fatalf("unexpected two heapStr ptr equal")
	}
	if s1.s != s2.s {
		t.Fatalf("unexpected two heapStr value not equal")
	}
	testWriteComparable(t, s1, s2)
	testWriteComparable(t, s1.s, s2.s)
	testWriteComparable(t, float32(0), negativeZero[float32]())
	testWriteComparable(t, float64(0), negativeZero[float64]())
	testWriteComparable(t, decimal64(9))
	testWriteComparable(t, decimal128(42))
	testWriteComparable(t, decimal64(0), math.Decimal64frombits(1<<63)) // +0 == -0
	testWriteComparableNoEqual(t, math.Decimal64NaN(), math.Decimal64NaN())
	testWriteComparableNoEqual(t, math.NaN(), math.NaN())
	testWriteComparableNoEqual(t, [2]string{"a", ""}, [2]string{"", "a"})
	testWriteComparableNoEqual(t, struct{ a, b string }{"foo", ""}, struct{ a, b string }{"", "foo"})
	testWriteComparableNoEqual(t, struct{ a, b any }{int(0), struct{}{}}, struct{ a, b any }{struct{}{}, int(0)})
}

func TestWriteComparableUnseeded(t *testing.T) {
	for _, n := range []int{0, 1, bufSize} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			s := strings.Repeat("a", n)
			var h1, h2 Hash
			h1.WriteString(s)
			WriteComparable(&h1, 123)

			h2.SetSeed(h1.Seed())
			h2.WriteString(s)
			WriteComparable(&h2, 123)

			if got, want := h1.Sum64(), h2.Sum64(); got != want {
				t.Errorf("unseeded hash = %#x, want %#x", got, want)
			}
		})
	}
}

func testWriteComparableNoEqual[T comparable](t *testing.T, v1, v2 T) {
	seed := MakeSeed()
	h1 := Hash{}
	h2 := Hash{}
	h1.seed, h2.seed = seed, seed
	WriteComparable(&h1, v1)
	WriteComparable(&h2, v2)
	if h1.Sum64() == h2.Sum64() {
		t.Fatalf("WriteComparable(seed, %v) == WriteComparable(seed, %v)", v1, v2)
	}

}

func testWriteComparable[T comparable](t *testing.T, v T, v2 ...T) {
	t.Run(reflect.TypeFor[T]().String(), func(t *testing.T) {
		var a, b T = v, v
		if len(v2) != 0 {
			b = v2[0]
		}
		var pa *T = &a
		h1 := Hash{}
		h2 := Hash{}
		h1.seed = MakeSeed()
		h2.seed = h1.seed
		WriteComparable(&h1, a)
		WriteComparable(&h2, b)
		if h1.Sum64() != h2.Sum64() {
			t.Fatalf("WriteComparable(h, %v) != WriteComparable(h, %v)", a, b)
		}
		WriteComparable(&h1, pa)
		old := h1.Sum64()
		stackGrow(8192)
		WriteComparable(&h2, pa)
		new := h2.Sum64()
		if old != new {
			t.Fatal("WriteComparable(seed, ptr) != WriteComparable(seed, ptr)")
		}
	})
}

func TestComparableShouldPanic(t *testing.T) {
	s := []byte("s")
	a := any(s)
	defer func() {
		e := recover()
		err, ok := e.(error)
		if !ok {
			t.Fatalf("Comaparable(any([]byte)) should panic")
		}
		want := "hash of unhashable type []uint8"
		if s := err.Error(); !strings.Contains(s, want) {
			t.Fatalf("want %s, got %s", want, s)
		}
	}()
	Comparable(MakeSeed(), a)
}

func TestWriteComparableNoncommute(t *testing.T) {
	seed := MakeSeed()
	var h1, h2 Hash
	h1.SetSeed(seed)
	h2.SetSeed(seed)

	h1.WriteString("abc")
	WriteComparable(&h1, 123)
	WriteComparable(&h2, 123)
	h2.WriteString("abc")

	if h1.Sum64() == h2.Sum64() {
		t.Errorf("WriteComparable and WriteString unexpectedly commute")
	}
}

func TestComparableAllocations(t *testing.T) {
	if asan.Enabled {
		t.Skip("skip allocation test under -asan")
	}
	seed := MakeSeed()
	x := heapStr(t)
	allocs := testing.AllocsPerRun(10, func() {
		s := "s" + x
		Comparable(seed, s)
	})
	if allocs > 0 {
		t.Errorf("got %v allocs, want 0", allocs)
	}

	type S struct {
		a int
		b string
	}
	allocs = testing.AllocsPerRun(10, func() {
		s := S{123, "s" + x}
		Comparable(seed, s)
	})
	if allocs > 0 {
		t.Errorf("got %v allocs, want 0", allocs)
	}
}

// Make sure a Hash implements the hash.Hash and hash.Hash64 interfaces.
var _ hash.Hash = &Hash{}
var _ hash.Hash64 = &Hash{}

func TestHashInterface(t *testing.T) {
	testhash.TestHash(t, func() hash.Hash { return new(Hash) })
}

func benchmarkSize(b *testing.B, size int) {
	h := &Hash{}
	buf := make([]byte, size)
	s := string(buf)

	b.Run("Write", func(b *testing.B) {
		b.SetBytes(int64(size))
		for i := 0; i < b.N; i++ {
			h.Reset()
			h.Write(buf)
			h.Sum64()
		}
	})

	b.Run("Bytes", func(b *testing.B) {
		b.SetBytes(int64(size))
		seed := h.Seed()
		for i := 0; i < b.N; i++ {
			Bytes(seed, buf)
		}
	})

	b.Run("String", func(b *testing.B) {
		b.SetBytes(int64(size))
		seed := h.Seed()
		for i := 0; i < b.N; i++ {
			String(seed, s)
		}
	})
}

func BenchmarkHash(b *testing.B) {
	sizes := []int{4, 8, 16, 32, 64, 256, 320, 1024, 4096, 16384}
	for _, size := range sizes {
		b.Run(fmt.Sprint("n=", size), func(b *testing.B) {
			benchmarkSize(b, size)
		})
	}
}

func benchmarkComparable[T comparable](b *testing.B, v T) {
	b.Run(reflect.TypeFor[T]().String(), func(b *testing.B) {
		seed := MakeSeed()
		for i := 0; i < b.N; i++ {
			Comparable(seed, v)
		}
	})
}

func BenchmarkComparable(b *testing.B) {
	type testStruct struct {
		i int
		u uint
		b bool
		f float64
		p *int
		a any
	}
	benchmarkComparable(b, int64(2))
	benchmarkComparable(b, uint64(8))
	benchmarkComparable(b, uintptr(12))
	benchmarkComparable(b, any("s"))
	benchmarkComparable(b, "s")
	benchmarkComparable(b, true)
	benchmarkComparable(b, new(float64))
	benchmarkComparable(b, float64(9))
	benchmarkComparable(b, complex128(9i+1))
	benchmarkComparable(b, struct{}{})
	benchmarkComparable(b, testStruct{i: 9, u: 1, b: true, f: 9.9, p: new(int), a: 1})
}

// TestComparableDecimal checks that Comparable and WriteComparable hash a
// decimal as map semantics (==) compares it: every member of a class below is
// ==, so all must hash equal, whatever the sign of a zero, the cohort member,
// or the encoding form. Members of different classes are not ==. Decimals
// have no pure-Go hash path in this package; they reach the runtime's map
// hasher, which uses the kernel's canonical hash key.
func TestComparableDecimal(t *testing.T) {
	pack64 := func(neg bool, coeff uint64, exp int) decimal64 {
		x, ok := bid.Pack64(neg, coeff, exp)
		if !ok {
			t.Fatalf("Pack64(%v, %d, %d) failed", neg, coeff, exp)
		}
		return math.Decimal64frombits(x)
	}
	pack128 := func(neg bool, coeff uint64, exp int) decimal128 {
		x, ok := bid.Pack128(neg, bid.Uint128{Lo: coeff}, exp)
		if !ok {
			t.Fatalf("Pack128(%v, %d, %d) failed", neg, coeff, exp)
		}
		return math.Decimal128frombits(x.Hi, x.Lo)
	}
	// Non-canonical encodings read as zero: a first-form coefficient above
	// 10^16-1 (or 10^34-1), and every second-form encoding.
	nonCanonZero64 := func(neg bool, biasedExp uint64) decimal64 {
		bits := bid.MaxCoeff64 + 1 | biasedExp<<bid.ExpShift64
		if neg {
			bits |= bid.SignMask64
		}
		return math.Decimal64frombits(bits)
	}
	form2Zero64 := math.Decimal64frombits(bid.Form2Mask64 | 5<<bid.Form2ExpShift64)
	nonCanonZero128 := func(neg bool, biasedExp uint64) decimal128 {
		hi := bid.MaxCoeff128Hi + 1 | biasedExp<<bid.ExpShift128
		if neg {
			hi |= bid.SignMask128Hi
		}
		return math.Decimal128frombits(hi, 0)
	}
	form2Zero128 := math.Decimal128frombits(bid.Form2Mask128Hi|9<<bid.Form2ExpShift128, 7)

	classes64 := [][]decimal64{
		{pack64(false, 0, 0), pack64(true, 0, 0), pack64(false, 0, -398), pack64(true, 0, 369),
			nonCanonZero64(false, 3), nonCanonZero64(true, 700), form2Zero64},
		{pack64(false, 15, -1), pack64(false, 150, -2), pack64(false, 1500000000000000, -15)},
		{pack64(true, 15, -1), pack64(true, 150, -2)},
		{pack64(false, 1, 0), pack64(false, 10, -1), pack64(false, 1000000000000000, -15)},
		{pack64(false, 1, 1), pack64(false, 10, 0), pack64(false, 100, -1)},
		{pack64(false, 1, 369), pack64(false, 10, 368)},
		{math.Decimal64Inf(1), math.Decimal64frombits(bid.Inf64 | 1<<50)},
		{math.Decimal64Inf(-1), math.Decimal64frombits(bid.Inf64 | bid.SignMask64 | 1<<50)},
	}
	classes128 := [][]decimal128{
		{pack128(false, 0, 0), pack128(true, 0, 0), pack128(false, 0, -6176), pack128(true, 0, 6111),
			nonCanonZero128(false, 3), nonCanonZero128(true, 9000), form2Zero128},
		{pack128(false, 15, -1), pack128(false, 150, -2), pack128(false, 15000000000000000, -16)},
		{pack128(true, 15, -1), pack128(true, 150, -2)},
		{pack128(false, 1, 0), pack128(false, 10, -1)},
		{pack128(false, 1, 1), pack128(false, 10, 0)},
		{pack128(false, 1, 6111), pack128(false, 10, 6110)},
		{decimal128(math.Decimal64Inf(1)), math.Decimal128frombits(bid.Inf128Hi|1<<40, 5)},
		{decimal128(math.Decimal64Inf(-1)), math.Decimal128frombits(bid.Inf128Hi|bid.SignMask128Hi|1<<40, 5)},
	}

	type S64 [1]decimal64
	type S128 [1]decimal128
	// check verifies the hash of a and b. Equal values must hash equal
	// (correctness). Unequal values should hash differently (quality, up to a
	// 2^-64 collision chance); that half runs only when distinct is set.
	check := func(t *testing.T, a, b any, distinct bool, hash func(Seed, any) uint64) {
		t.Helper()
		wantEq := a == b
		seed := MakeSeed()
		gotEq := hash(seed, a) == hash(seed, b)
		if wantEq && !gotEq {
			t.Errorf("%#v == %#v but their hashes differ", a, b)
		}
		if !wantEq && gotEq && distinct {
			t.Errorf("%#v != %#v but their hashes are equal", a, b)
		}
	}
	comparable := func(seed Seed, v any) uint64 {
		switch v := v.(type) {
		case decimal64:
			return Comparable(seed, v)
		case decimal128:
			return Comparable(seed, v)
		case S64:
			return Comparable(seed, v)
		case S128:
			return Comparable(seed, v)
		}
		panic("unexpected type")
	}
	written := func(seed Seed, v any) uint64 {
		h := Hash{}
		h.SetSeed(seed)
		switch v := v.(type) {
		case decimal64:
			WriteComparable(&h, v)
		case decimal128:
			WriteComparable(&h, v)
		case S64:
			WriteComparable(&h, v)
		case S128:
			WriteComparable(&h, v)
		default:
			panic("unexpected type")
		}
		return h.Sum64()
	}
	hashers := []struct {
		name string
		f    func(Seed, any) uint64
	}{{"Comparable", comparable}, {"WriteComparable", written}}

	for _, hs := range hashers {
		for _, distinct := range []bool{false, true} {
			name := hs.name + "/Agree"
			if distinct {
				name = hs.name + "/Distinct"
			}
			t.Run(name, func(t *testing.T) {
				// Bare values and one-element arrays hash alike by design, so
				// compare within each Go type only.
				var bare64, arr64, bare128, arr128 []any
				for _, c := range classes64 {
					for _, x := range c {
						bare64 = append(bare64, x)
						arr64 = append(arr64, S64{x})
					}
				}
				for _, c := range classes128 {
					for _, x := range c {
						bare128 = append(bare128, x)
						arr128 = append(arr128, S128{x})
					}
				}
				for _, all := range [][]any{bare64, arr64, bare128, arr128} {
					for i, a := range all {
						for _, b := range all[i:] {
							check(t, a, b, distinct, hs.f)
						}
					}
				}
			})
		}
	}
}

// TestComparableDecimalStruct checks a decimal inside a struct, whose hash
// function the compiler generates: the members of a cohort and the two zeros
// are one key, and a different field or value is another.
func TestComparableDecimalStruct(t *testing.T) {
	type key struct {
		n int
		d decimal64
	}
	seed := MakeSeed()
	neg0 := -decimal64(0)
	if Comparable(seed, key{1, 1.5}) != Comparable(seed, key{1, 1.50}) {
		t.Errorf("cohort members 1.5 and 1.50 hash differently in a struct")
	}
	if Comparable(seed, key{1, 0}) != Comparable(seed, key{1, neg0}) {
		t.Errorf("+0 and -0 hash differently in a struct")
	}
	if Comparable(seed, key{1, 1.5}) == Comparable(seed, key{1, -1.5}) {
		t.Errorf("1.5 and -1.5 hash alike in a struct")
	}
	if Comparable(seed, key{1, 1.5}) == Comparable(seed, key{2, 1.5}) {
		t.Errorf("a different integer field hashes alike in a struct")
	}
}
