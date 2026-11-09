// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"internal/bid"
	"math/big"
	dec "math/decimal"
	"math/rand/v2"
	"testing"
)

// These boundaries keep the public calls in compiled code while allowing
// constants to reach their implementation. The variable cases exercise the
// same lowering without constant mode or quantum facts.
//
//go:noinline
func compiledQuant64(x, y decimal64, m dec.RoundingMode) decimal64 { return dec.QuantizeMode(x, y, m) }

//go:noinline
func compiledQuant128(x, y decimal128, m dec.RoundingMode) decimal128 {
	return dec.QuantizeMode(x, y, m)
}

//go:noinline
func compiledCent64(x decimal64) decimal64 { return dec.Quantize(x, decimal64(0.01)) }

//go:noinline
func compiledCent128(x decimal128) decimal128 { return dec.Quantize(x, decimal128(0.01)) }

type roundingNamed64 decimal64
type roundingNamed128 decimal128

//go:noinline
func compiledAway64(x roundingNamed64) roundingNamed64 {
	return dec.QuantizeMode(x, roundingNamed64(0.01), dec.ToNearestAway)
}

//go:noinline
func compiledAway128(x roundingNamed128) roundingNamed128 {
	return dec.QuantizeMode(x, roundingNamed128(0.01), dec.ToNearestAway)
}

//go:noinline
func compiledIntegral64(x decimal64) [5]decimal64 {
	return [5]decimal64{dec.RoundToEven(x), dec.Round(x), dec.Trunc(x), dec.Ceil(x), dec.Floor(x)}
}

//go:noinline
func compiledIntegral128(x decimal128) [5]decimal128 {
	return [5]decimal128{dec.RoundToEven(x), dec.Round(x), dec.Trunc(x), dec.Ceil(x), dec.Floor(x)}
}

func TestCompiledRounding(t *testing.T) {
	for _, f := range both {
		t.Run(f.name, func(t *testing.T) {
			cent := f.encode(value{kind: finite, coeff: big.NewInt(1), exp: -2})
			check := func(x, y bits, m bid.Mode) {
				t.Helper()
				var q, c, away bits
				var integral [5]bits
				if f == dec64 {
					q = b64(compiledQuant64(d64(x), d64(y), dec.RoundingMode(m)))
					c = b64(compiledCent64(d64(x)))
					away = b64(decimal64(compiledAway64(roundingNamed64(d64(x)))))
					for i, v := range compiledIntegral64(d64(x)) {
						integral[i] = b64(v)
					}
				} else {
					q = b128(compiledQuant128(d128(x), d128(y), dec.RoundingMode(m)))
					c = b128(compiledCent128(d128(x)))
					away = b128(decimal128(compiledAway128(roundingNamed128(d128(x)))))
					for i, v := range compiledIntegral128(d128(x)) {
						integral[i] = b128(v)
					}
				}
				if want := (reference{}).quantize(f, x, y, m); q != want {
					t.Fatalf("quantize %x %x mode %d: got %x want %x", x, y, m, q, want)
				}
				if want := (reference{}).quantize(f, x, cent, bid.ToNearestEven); c != want {
					t.Fatalf("cent %x: got %x want %x", x, c, want)
				}
				if want := (reference{}).quantize(f, x, cent, bid.ToNearestAway); away != want {
					t.Fatalf("named away %x: got %x want %x", x, away, want)
				}
				for i, got := range integral {
					if want := (reference{}).roundToIntegral(f, x, bid.Mode(i)); got != want {
						t.Fatalf("integral %x mode %d: got %x want %x", x, i, got, want)
					}
				}
			}
			r := rand.New(rand.NewPCG(*seedFlag, streamSeed("compiled-rounding")))
			for i := 0; i < numCases(); i++ {
				x, y := bits{Lo: r.Uint64()}, bits{Lo: r.Uint64()}
				if f == dec128 {
					x.Hi, y.Hi = r.Uint64(), r.Uint64()
				}
				check(x, y, bid.Mode(r.Uint32()))
				// Dense bounded inputs, including random remainders and word limits.
				coeff := r.Uint64()
				if f == dec64 {
					coeff %= bid.MaxCoeff64 + 1
				}
				exp := -32 + int(r.Uint32N(38))
				target := exp + int(r.Uint32N(42)) - 20
				x = f.encode(value{kind: finite, neg: r.Uint32N(2) == 0, coeff: new(big.Int).SetUint64(coeff), exp: exp})
				y = f.encode(value{kind: finite, coeff: big.NewInt(1), exp: target})
				check(x, y, bid.Mode(r.Uint32N(6)))
			}
			// Force every fast-path boundary: first/second coefficient forms, word
			// limit, all quantum differences around the table, signed zero, ties,
			// carry and both edges of the exponent range. All invalid byte modes
			// must retain the nearest-even default.
			for _, n := range []uint64{0, 1, 5, 9, 10, 15, 25, 99, 100, 1<<53 - 1, 1 << 53, bid.MaxCoeff64, ^uint64(0)} {
				c := new(big.Int).SetUint64(n)
				if c.Cmp(pow10(f.prec)) >= 0 {
					continue
				}
				for _, e := range []int{f.minExp, -20, -19, -3, -2, -1, 0, 1, f.maxExp} {
					for _, neg := range []bool{false, true} {
						x := f.encode(value{kind: finite, neg: neg, coeff: c, exp: e})
						for _, gap := range []int{-20, -1, 0, 1, 2, 18, 19, 20} {
							target := e + gap
							if target < f.minExp || target > f.maxExp {
								continue
							}
							y := f.encode(value{kind: finite, coeff: big.NewInt(1), exp: target})
							for _, m := range []bid.Mode{0, 1, 2, 3, 4, 5, 255} {
								check(x, y, m)
							}
						}
					}
				}
			}
			for m := 0; m < 256; m++ {
				check(f.encode(value{kind: finite, coeff: big.NewInt(125), exp: -3}), cent, bid.Mode(m))
			}
		})
	}
}

var compiledRoundSink64 decimal64
var compiledRoundSink128 decimal128

func BenchmarkCompiledRounding(b *testing.B) {
	for _, tc := range []struct{ name, text string }{
		{"word", "12345.675"}, {"exact", "12345.67"}, {"tie", "2.345"},
		{"wide", "123456789012345.5"}, {"far", "1E-100"}, {"special", "NaN"},
	} {
		b.Run(tc.name+"/64", func(b *testing.B) {
			x, _ := bid.Parse64(tc.text, bid.ToNearestEven)
			v := d64(bits{Lo: x})
			b.ReportAllocs()
			for b.Loop() {
				compiledRoundSink64 = compiledCent64(v)
			}
		})
		b.Run(tc.name+"/128", func(b *testing.B) {
			x, _ := bid.Parse128(tc.text, bid.ToNearestEven)
			v := d128(x)
			b.ReportAllocs()
			for b.Loop() {
				compiledRoundSink128 = compiledCent128(v)
			}
		})
	}
}

// Evaluation must occur once, in source order, even on a guarded fallback.
func TestCompiledRoundingEvaluation(t *testing.T) {
	for _, text := range []string{"1.235", "NaN"} {
		raw, _ := bid.Parse128(text, bid.ToNearestEven)
		order := 0
		operand := func(want int, x decimal128) decimal128 {
			if order != want {
				t.Fatalf("order %d want %d", order, want)
			}
			order++
			return x
		}
		mode := func() dec.RoundingMode {
			if order != 2 {
				t.Fatal(order)
			}
			order++
			return dec.ToNearestEven
		}
		got := dec.QuantizeMode(operand(0, d128(raw)), operand(1, decimal128(0.01)), mode())
		if order != 3 || b128(got) != (reference{}).quantize(dec128, raw, dec128.encode(value{kind: finite, coeff: big.NewInt(1), exp: -2}), bid.ToNearestEven) {
			t.Fatalf("evaluation result %x order %d", b128(got), order)
		}
	}
}

//go:noinline
func compiledTrunc128(x decimal128) decimal128 { return dec.Trunc(x) }

//go:noinline
func compiledEven128(x decimal128) decimal128 { return dec.RoundToEven(x) }

func BenchmarkCompiledRoundingExtended(b *testing.B) {
	values := []decimal128{12345.675, -2.345, 0.0001, 123456789012345678901234567890.1234}
	q := decimal128(0.01)
	b.Run("variable", func(b *testing.B) {
		var x decimal128
		for i := 0; b.Loop(); i++ {
			x = compiledQuant128(values[i%len(values)], q, dec.RoundingMode(i%5))
		}
		compiledRoundSink128 = x
	})
	b.Run("wide", func(b *testing.B) {
		for b.Loop() {
			compiledRoundSink128 = compiledCent128(values[3])
		}
	})
	b.Run("integralEven", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			compiledRoundSink128 = compiledEven128(values[i%len(values)])
		}
	})
	b.Run("integralTrunc", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			compiledRoundSink128 = compiledTrunc128(values[i%len(values)])
		}
	})
	b.Run("chain", func(b *testing.B) {
		x := decimal128(1)
		for b.Loop() {
			x = compiledCent128(x + decimal128(0.015))
		}
		compiledRoundSink128 = x
	})
}

// This generic dispatch resembles ordinary callers selecting a rounding rule.
// It must remain correct whether its caller supplies a constant or a value.
func compiledIntegralDispatch[D dec.Decimal](x D, m dec.RoundingMode) D {
	switch m {
	case dec.ToNearestEven:
		return dec.RoundToEven(x)
	case dec.ToNearestAway:
		return dec.Round(x)
	case dec.ToZero:
		return dec.Trunc(x)
	case dec.ToPositiveInf:
		return dec.Ceil(x)
	case dec.ToNegativeInf:
		return dec.Floor(x)
	default:
		panic("invalid rounding mode")
	}
}

var compiledCeilFunction = dec.Ceil[roundingNamed64]
var compiledFloorFunction = dec.Floor[roundingNamed128]

func TestCompiledRoundingCallForms(t *testing.T) {
	for _, text := range []string{"-0.00", "0.0001", "-0.0001", "12.5", "-12.5", "1E+100", "1234567890123456789012345678901234", "NaN", "-Infinity"} {
		a, _ := bid.Parse64(text, bid.ToNearestEven)
		x := bits{Lo: a}
		y, _ := bid.Parse128(text, bid.ToNearestEven)
		if got, want := b64(decimal64(compiledCeilFunction(roundingNamed64(d64(x))))), (reference{}).roundToIntegral(dec64, x, bid.TowardPositive); got != want {
			t.Fatalf("function value64 %s: got %x want %x", text, got, want)
		}
		if got, want := b128(decimal128(compiledFloorFunction(roundingNamed128(d128(y))))), (reference{}).roundToIntegral(dec128, y, bid.TowardNegative); got != want {
			t.Fatalf("function value128 %s: got %x want %x", text, got, want)
		}
		for m := dec.ToNearestEven; m <= dec.ToNegativeInf; m++ {
			if got, want := b128(compiledIntegralDispatch(d128(y), m)), (reference{}).roundToIntegral(dec128, y, bid.Mode(m)); got != want {
				t.Fatalf("dispatch %s mode %d: got %x want %x", text, m, got, want)
			}
		}
		var deferred decimal64
		func() { defer func(v decimal64) { deferred = dec.Trunc(v) }(d64(x)) }()
		if got, want := b64(deferred), (reference{}).roundToIntegral(dec64, x, bid.TowardZero); got != want {
			t.Fatalf("defer %s: got %x want %x", text, got, want)
		}
		c := make(chan decimal128)
		go func(v decimal128) { c <- dec.RoundToEven(v) }(d128(y))
		if got, want := b128(<-c), (reference{}).roundToIntegral(dec128, y, bid.ToNearestEven); got != want {
			t.Fatalf("goroutine %s: got %x want %x", text, got, want)
		}
	}
}
