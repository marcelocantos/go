# Digit conversion without the 10,000-entry table

Punch list 2 (tune formatting), first question: can `quadTab`, the
10,000-entry table of four ASCII digits (40 KB) that `eightQuads` uses to
convert eight digits for `PutShort128` and `PutLong128`, be replaced by a
conversion with no table, as decimal128-go and its siblings do
(`render8DigitsBeforeIndex`), keeping the eight characters in a register
for one store? Branch ubd-format-notable, off ubd-level4. Apple M3 Pro,
2026-11-23.

The history (internal/bid/doc.go, "Digit conversion"): the table did not
replace a table-free conversion directly. `eightASCII` (three packed
multiplications, each waiting for the last) gave way to `eightPairs`
(pairs side by side, then a 100-entry table; money 23.5 → 21 cycles,
34 digits 68 → 52), and that to `quadTab` (one quotient, two loads;
money 21 → 19.4 cycles, 34 digits 52 → 37–44; apps 0.3–3% faster).

## Variants

All give `eightASCII`'s layout (most significant digit in the low byte,
stored as one word) and agree with it on every v below 10^8 (exhaustive
test).

| | conversion | depth |
|---|---|---|
| T | `quadTab`: v / 10^4, then two table loads | 1 multiply + loads |
| A | `eightASCII`: 10^4 split, 100 split, 10 split, packed lanes | 3 multiplies |
| P | the four pairs from independent quotients of v (v/100, v/10^4, v/10^6), packed, then one packed tens split | 2 multiplies |
| D | the eight digits from seven independent quotients of v, each a multiply-subtract, packed by shifts | 1 multiply + 1 multiply-subtract |
| U | decimal128-go's cascade with its M and S constants (10^4 by multiply-high, then 10^2 and 10 on 32-bit values), packed into a register | 3 levels, 14 divisions |

The M and S constants do not change the cost: each division is one
multiply and one shift whichever constants are used. P and D need
constants exact for v below 10^8 (k = 32 for 100, 47 for 10^6, 50 for
10^7); the 40-bit ones first tried failed the exhaustive test.

## The conversion alone

ns per eight digits; arm64 instructions in a wrapper (T, A and P inline;
D and U are over the inlining budget):

| | T | A | P | D | U |
|---|---|---|---|---|---|
| throughput | 0.76 | 1.48 | 1.56 | 3.07 | 3.27 |
| latency (chained) | 5.69 | 9.90 | 8.76 | 8.54 | 10.20 |
| arm64 instructions | 35 (bounds checks) | 23 | 31 | 51 | 47 |

## In the formatters (BenchmarkKernel, medians of 7)

| row | T | A | P | A vs T | P vs T |
|---|---|---|---|---|---|
| PutShort/128/short | 4.85 | 5.96 | 5.87 | +22.9% | +21.0% |
| Append/128/short | 5.69 | 6.66 | 6.83 | +17.2% | +20.2% |
| Append/128/shuffled | 7.21 | 8.30 | 8.26 | +15.1% | +14.4% |
| Append/128/mixed | 9.95 | 12.45 | 12.69 | +25.1% | +27.5% |
| Append/128/amount11 | 6.84 | 7.93 | 8.48 | +15.9% | +24.0% |
| PutLong/128/long | 7.83 | 10.42 | 10.54 | +33.2% | +34.7% |
| Append/128/long | 8.71 | 11.54 | 11.69 | +32.5% | +34.2% |
| Append/128/digits34 | 9.91 | 14.35 | 14.75 | +44.7% | +48.8% |
| AppendSci/128/mixed | 16.12 | 16.20 | 16.21 | +0.5% | +0.6% |

arm64 instructions: `PutShort128` 128 (T, A) and 144 (P); `PutLong128`
464 (T) and 496 (A, P).

## Apps (level 2, the three builds interleaved, best of 7)

Outputs equal for every variant.

| app | T | A | P | A vs T | P vs T | medians A / P vs T |
|---|---|---|---|---|---|---|
| telco | 28.9 | 29.9 | 30.2 | +3.5% | +4.4% | +2.4% / +3.3% |
| tax nc | 734.2 | 757.5 | 761.6 | +3.2% | +3.7% | +4.1% / +4.0% |
| tax fed | 2290.2 | 2335.0 | 2325.8 | +2.0% | +1.6% | +1.3% / +1.4% |
| tax ca | 487.4 | 495.6 | 496.7 | +1.7% | +1.9% | +2.8% / +3.2% |
| risk | 5791.6 | 5854.1 | 5860.4 | +1.1% | +1.2% | −0.4% / +0.7% |
| banking | 638.2 | 643.6 | 642.1 | +0.9% | +0.6% | +0.4% / +0.5% |
| euro | 873.6 | 880.4 | 879.4 | +0.8% | +0.7% | +0.1% / +0.2% |

Without the table, telco's binary is 49,568 bytes smaller (2,088,210 →
2,038,642).

## Result

The table stays. Its loads have a shorter dependency chain than any
table-free form: one multiply and two loads, against two or three
multiplies one after another. That makes formatting 15–49% faster, and
the apps, where the table shares the cache with the rest of the program,
are 0.6–4.4% faster with it, most where they format most (telco, tax).
The table-free forms cost 40 KB less data. The cost of formatting that
is left lies outside the eight-digit conversion: strconv's dispatch, the
layout of the point and exponent, and the allocation of the string.
