// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

// TestUnpackDigits compares the unformatted parts with the independent decoder and
// big.Int decimal conversion, including raw noncanonical and special inputs.
func TestUnpackDigits(t *testing.T) {
	for _, f := range both {
		g := &gen{f: f, r: rand.New(rand.NewPCG(*seedFlag, streamSeed("digits-"+f.name)))}
		for i := range numCases() {
			g.regime = regime(i % int(numRegimes))
			x := bits{Lo: g.r.Uint64()}
			if f == dec128 {
				x.Hi = g.r.Uint64()
			}
			if i&1 != 0 {
				x = f.encode(g.finite())
			}
			v := f.decode(x)
			wantKind, want := bid.KindFinite, ""
			switch v.kind {
			case infinite:
				wantKind = bid.KindInf
			case notANumber:
				wantKind = bid.KindNaN
			default:
				if v.coeff.Sign() != 0 {
					want = v.coeff.String()
				}
			}
			var buf64 [bid.Precision64]byte
			var buf128 [bid.Precision128]byte
			for j := range buf64 {
				buf64[j] = '?'
			}
			for j := range buf128 {
				buf128[j] = '?'
			}
			var kind bid.Kind
			var neg bool
			var n, exp int
			var buf []byte
			if f == dec64 {
				kind, neg, n, exp = bid.UnpackDigits64(&buf64, x.Lo)
				buf = buf64[:]
			} else {
				kind, neg, n, exp = bid.UnpackDigits128(&buf128, x)
				buf = buf128[:]
			}
			if kind != wantKind || neg != v.neg || exp != v.exp || n < 0 || n > len(buf) {
				t.Fatalf("%s UnpackDigits(%x): got (%v, %t, %d, %d), want (%v, %t, %d, %d)", f.name, x, kind, neg, n, exp, wantKind, v.neg, len(want), v.exp)
			}
			if string(buf[len(buf)-n:]) != want {
				t.Fatalf("%s UnpackDigits(%x) = %q, want %q", f.name, x, buf[len(buf)-n:], want)
			}
			for _, c := range buf[:len(buf)-n] {
				if c != '?' {
					t.Fatalf("%s UnpackDigits(%x) overwrote the buffer before its final %d digits", f.name, x, n)
				}
			}
		}
	}
}

// Exact chunk multiples force the reciprocal estimate's correction. Adjacent
// remainders, word boundaries and the largest coefficient exercise the product
// carries. Check the production digit API against independent big.Int text.
func TestUnpackDigitsSplitBoundaries(t *testing.T) {
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(19), nil)
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(bid.Precision128), nil)
	quotients := []*big.Int{new(big.Int), big.NewInt(1)}
	for _, radix := range []int64{2, 10} {
		for power := big.NewInt(1); power.Cmp(new(big.Int).Quo(limit, base)) < 0; power.Mul(power, big.NewInt(radix)) {
			for _, delta := range []int64{-1, 0, 1} {
				quotients = append(quotients, new(big.Int).Add(power, big.NewInt(delta)))
			}
		}
	}
	quotients = append(quotients, new(big.Int).Sub(new(big.Int).Quo(limit, base), big.NewInt(1)))
	coefficients := []*big.Int{new(big.Int).Sub(limit, big.NewInt(1))}
	wordBoundary := new(big.Int).Lsh(big.NewInt(1), 64)
	for _, delta := range []int64{-1, 0, 1} {
		coefficients = append(coefficients, new(big.Int).Add(wordBoundary, big.NewInt(delta)))
	}
	for _, q := range quotients {
		for _, r := range []*big.Int{new(big.Int), big.NewInt(1), new(big.Int).Rsh(new(big.Int).Set(base), 1), new(big.Int).Sub(base, big.NewInt(1))} {
			coefficients = append(coefficients, new(big.Int).Add(new(big.Int).Mul(q, base), r))
		}
	}
	for _, c := range coefficients {
		if c.Sign() < 0 || c.Cmp(limit) >= 0 {
			continue
		}
		want := c.String()
		if c.Sign() == 0 {
			want = ""
		}
		coeff := bid.Uint128{Hi: new(big.Int).Rsh(new(big.Int).Set(c), 64).Uint64(), Lo: c.Uint64()}
		for _, neg := range []bool{false, true} {
			for _, exp := range []int{bid.MinExp128, 0, bid.MaxExp128} {
				x, ok := bid.Pack128(neg, coeff, exp)
				if !ok {
					t.Fatalf("Pack128(%s, %d) rejected a bounded coefficient", c, exp)
				}
				var buf [bid.Precision128]byte
				kind, gotNeg, n, gotExp := bid.UnpackDigits128(&buf, x)
				if kind != bid.KindFinite || gotNeg != neg || gotExp != exp || n != len(want) || string(buf[len(buf)-n:]) != want {
					t.Fatalf("UnpackDigits128(%s, %d, %v): kind=%v sign=%v n=%d exp=%d digits=%q", c, exp, neg, kind, gotNeg, n, gotExp, buf)
				}
			}
		}
	}
}

// BenchmarkUnpackDigitsCases keeps the word shortcut's misses visible separately
// from the mixed API ratchet. Rotate operands so no fixed value is folded.
func BenchmarkUnpackDigitsCases(b *testing.B) {
	for _, f := range both {
		b.Run(f.name, func(b *testing.B) {
			r := rand.New(rand.NewPCG(541, streamSeed("digit-cases-"+f.name)))
			for _, name := range []string{"word", "wide", "zero", "special", "raw"} {
				var xs [64]bits
				for i := range xs {
					switch name {
					case "word":
						c := new(big.Int).SetUint64(1 + r.Uint64()%1_000_000_000)
						xs[i] = f.encode(value{kind: finite, neg: i&1 != 0, coeff: c, exp: i%15 - 7})
					case "wide":
						c := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(f.prec)), nil)
						c.Sub(c, big.NewInt(int64(i+1)))
						xs[i] = f.encode(value{kind: finite, neg: i&1 != 0, coeff: c, exp: i%15 - 7})
					case "zero":
						xs[i] = f.encode(value{kind: finite, neg: i&1 != 0, coeff: new(big.Int), exp: i%15 - 7})
					case "special":
						xs[i] = f.encode(value{kind: []kind{infinite, notANumber}[i%2], neg: i&2 != 0})
					case "raw":
						xs[i] = bits{Hi: r.Uint64(), Lo: r.Uint64()}
					}
				}
				b.Run(name, func(b *testing.B) {
					var buf64 [bid.Precision64]byte
					var buf128 [bid.Precision128]byte
					sum := 0
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						x := xs[i%len(xs)]
						var n int
						if f == dec64 {
							_, _, n, _ = bid.UnpackDigits64(&buf64, x.Lo)
						} else {
							_, _, n, _ = bid.UnpackDigits128(&buf128, x)
						}
						sum += n
					}
					asInt = sum
				})
			}
		})
	}
}

// UnpackDigits has its own word shortcut. Probe Append128 separately so a
// faster wide split cannot hide costs on cSplit128's two word-sized paths.
func BenchmarkAppend128Word(b *testing.B) {
	const count = 64
	for _, c := range []struct {
		name  string
		first uint64
	}{{"small", 12345678}, {"large", ^uint64(0) - (count - 1)}} {
		var xs [count]bid.Uint128
		for i := range xs {
			xs[i], _ = bid.Pack128(i&1 != 0, bid.Uint128{Lo: c.first + uint64(i)}, i%15-7)
		}
		b.Run(c.name, func(b *testing.B) {
			var buf [bid.MaxLen128]byte
			sum := 0
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sum += len(bid.Append128(buf[:0], xs[i%len(xs)]))
			}
			asInt = sum
		})
	}
}
