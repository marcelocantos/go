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

# The string formed in its own bytes (2026-11-23)

decimal128-go allocates the result at its exact length, which it knows
before writing (it counts the digits first), writes the characters into
that allocation, and returns it as the string with no copy
(`withStringBuffer`, `unsafe.String`). `strconv.FormatDecimal` here
formatted into a stack buffer and returned `string(buf[:n])`:
`slicebytetostring` allocates and copies, after `appendDecimal`'s
dispatch. In telco's level-2 profile `slicebytetostring` was 26%
cumulative of the run against 11% for `appendDecimal`.

## What was built

- `internal/bid.ShortForm128(frac, c)`: `PutShort128` without its
  stores. It returns the two words `PutShort128` lays out and the
  form's length after the sign, and documents the stores, every one of
  which lies inside the form. `internal/bid` may not import `unsafe`, so
  the stores are the caller's.
- `strconv.formatDecimal` takes the format switch and the short test
  once, then:
  - a short value of one or two characters (0 to 99, −0 to −9) is a
    slice of a constant: no allocation;
  - any other short value is allocated at its exact length
    (`internal/bytealg.MakeNoZero`, which `strings.Builder` uses: no
    zeroing) and `PutShort128`'s stores are made into it, each through a
    pointer to its own bytes (`*(*uint32)(unsafe.Add(p, i))`, byte-swapped
    on big-endian targets); a form of fewer than four bytes stores its
    words to a scratch buffer by a selected address, as `PutShort128`
    does; the bytes become the string with `unsafe.String`;
  - every other value takes `appendDecimal`'s remaining paths
    (`appendDecimalRest`, split out of `appendDecimal`) with the words
    decoded once, and is copied as before.
- Long values keep the copy: going direct would allocate `PutLong128`'s
  42-byte room for results often 10–20 bytes long, and in the apps long
  values use 'f', which `PutLong128` does not serve (see below).

## Variants measured on the way

ns per call (`strconv.FormatDecimal`, 'g'; money is 7–8 characters):

| variant | money | 11-digit amount | bytes per money string |
|---|---|---|---|
| original | 13.4 | 14.6 | 8 |
| X1: both paths straight into `make([]byte, room)` | 11.1 | 14.2 | 16 |
| X2: no dispatch, but stack buffer and copy | 12.7 | 13.9 | 8 |
| X3: X1's short path only, the format checked once | 11.8 | 15.3 | 16 |
| X4: exact length, `MakeNoZero` | 12.2 | 15.5 | 8 |
| X4b: exact length, `make` (zeroing) | 12.9 | 15.5 | 8 |
| X6: X4 with one- and two-character results from constants | 11.7 | 15.4 | 8 |

Findings:

- Dispatch alone (X2) is about 5%; the copy and its call another 10–15%
  on short values.
- A 16-byte allocation (X1, X3) is barely dearer than a tiny one, but
  doubles the memory of a money string.
- `make` zeroes and packs a 3-byte string in 3 bytes where `MakeNoZero`
  rounds it to 8; the zeroing costs more than the packing saves.
- X3 and X4 made the tax apps 3–12% slower, although each was faster on
  every microbenchmark row. In compute mode the tax apps keep only the
  length of each string, and a tax form is full of zeros and single
  digits: the original's `string(buf[:n])` gave those the runtime's
  static one-byte strings, with no allocation; X3 and X4 allocated every
  one. Garbage collection was not the cause (one to three collections a
  pass in every variant), nor code placement (a control build, the
  original with a padding function before `formatDecimal`, moved tax by
  about 1%).

## Results (X6)

Microbenchmark, ns per call:

| class | original | X6 | bytes, allocations |
|---|---|---|---|
| money, 'g' | 13.42 | 11.98 | 8, 1 → 8, 1 |
| −money, 'g' | 13.76 | 11.98 | 8, 1 → 8, 1 |
| integers below 1000, 'g' | 13.50 | 12.30 | 3, 0 → 7, 0 |
| money, 'f' | 13.34 | 12.00 | 8, 1 → 8, 1 |
| 11-digit amount, 'g' | 14.61 | 15.4 | 16, 1 (no change) |
| 11-digit amount, 'f' | 23.68 | 23.9 | 16, 1 (no change) |
| 34 digits, 'g' | 19.34 | 19.9 | 48, 1 (no change) |

Apps at level 2, best of 11 interleaved, outputs equal:

| app | X6 vs original | median |
|---|---|---|
| telco | −10.1% | −10.0% |
| tax nc | −6.1% | −4.4% |
| tax fed | −0.6% | −1.5% |
| tax ca | −0.6% | +0.5% |
| risk | −0.3% | −0.9% |
| banking | +0.3% | +0.6% |
| euro | +0.6% | +0.2% |

arm64 instructions: `strconv.FormatDecimal` 64 → 48, and
`formatDecimal`, which `FormatDecimal` inlined, is now a function of 88;
`shortString` 124 (with `ShortForm128` 84, a call); `appendDecimal` 712
→ 192 and `appendDecimalRest` 536.

Correctness: `FormatDecimal` (the new paths) against `AppendDecimal`
(the old) on 2M random short values in 'f', 'g' and 'G' (signs, 0 to 9
places, zeros, small integers), natively, under `-race` (`checkptr`)
and for amd64; the strconv, math/decimal, fmt and internal/decimal
tests; `TestShortForm128` checks `ShortForm128`'s documented stores
against `PutShort128` for every coefficient below 10^5 and a sweep to
10^8 at 0 to 4 places of either sign; strconv builds for s390x
(big-endian).

Open:

- `internal/bid`'s `TestBenchBaselineShape` fails: the new row
  ShortForm/128/short has no sample in
  testdata/bench_baseline_darwin_arm64.txt, which is Marcelo's run on
  his M4 Max and has to be retaken there, by a deliberate commit, as the
  file says.
- Long values with 'f' (the apps' `String`) take the general path, 23.7
  ns against 14.6 for 'g' through `PutLong128`. Where the canonical
  form is plain notation, which is 'f''s output, `PutLong128` could
  serve 'f' too. That is the larger remaining gain for long values.

## Long values in the fixed form

Long values (coefficients of nine digits or more) formatted with 'f' at
precision -1, which is what the apps' `String` asks for, took the
general path: the digits unpacked a byte each and appended. 'g' had
`PutLong128`.

Counts first (one normal run of each app, a temporary counter in
strconv, since removed). Every app formats with 'f' at -1, but for euro's
10 'b':

| app | formatted | short path | long, e −4..5 | long, e ≥ 6 | long, other | n ≤ 8, not short |
|---|---|---|---|---|---|---|
| euro | 2,145 | 862 | 367 (17%) | 654 (30%) | 4 (e < −4) | 243 (11%) |
| banking | 2,085 | 1,846 | 18 (1%) | 121 (6%) | | 100 (5%) |
| tax fed | 57 | 50 | 7 | | | |
| tax nc | 37 | 37 | | | | |
| tax ca | 19 | 19 | | | | |
| telco | 1,000,003 | 1,000,002 | | 1 | | |
| risk | 0 | | | | | |

e is the exponent of the leading digit. In PutLong128's plain window
(exponent ≤ 0, e from −4 to 5) 'f' and 'g' are the same bytes; long
money of a million or more (e ≥ 6) is exponential in 'g', and was most
of euro's long values. So the fixed form goes past the window:

- `bid.PutLongFixed128` shares PutLong128's body (`putLong128`, with a
  flag) and writes plain notation for any exponent ≤ 0 and e ≥ −4,
  returning 0, with nothing written, for other values (trailing zeros,
  or below 0.0001), which take the general path.
- The point can now be as far as 33 digits in. Two changes make the
  plain layout hold there: a fourth word stored from the left (at 24,
  when the form has 32 characters), since the digits before the point
  reach past 23 unless the last word holds them; and the point's index
  is clamped, not masked to five bits. `TestLong128` now checks
  `PutLongFixed128` on all its values against the fixed form built from
  the digits; it fails with either change undone.
- `appendDecimalRest` sends 'f' and 'g' to the kernel; 'G', 'e' and 'E'
  keep the general path.

Microbenchmarks (M3 Pro, funcalign=4096, median of 9; bytes and
allocations unchanged):

| 'f', -1 | before ns | after ns | change |
|---|---|---|---|
| money (short path) | 12.05 | 12.05 | 0 |
| 11-digit amount | 26.36 | 18.66 | −29.2% |
| 20-digit amount | 28.75 | 20.45 | −28.9% |
| 34 digits | 34.98 | 20.96 | −40.1% |
| rate, 6 places | 27.92 | 17.85 | −36.1% |
| a million or more, cents | 26.16 | 18.62 | −28.8% |
| tax mix | 20.28 | 17.15 | −15.4% |

'g' is unchanged but for 34 digits, +2.0% (0.4 ns): the fourth store.

Apps at level 2, 11 rounds interleaved, outputs equal:

| app | before best | after best | change | median change |
|---|---|---|---|---|
| euro | 890.3 | 781.2 | −12.3% | −12.8% |
| banking | 636.7 | 638.5 | +0.3% | −0.7% |

The other apps format no long values (telco one), so were not rerun.

arm64 instructions: `PutLong128` 456 → `putLong128` 480 (the exported
wrappers inline into strconv); `appendDecimalRest` 536 → 600.

Correctness: `TestLong128` (above); strconv, math/decimal and fmt tests;
2M random long values, 9 to 34 digits, exponents from 8 places left of
the digits to 3 past them, through `FormatDecimal` and `AppendDecimal`
at five amounts of spare capacity, against the fixed form built from the
digits, natively, under `-race` and for amd64; internal/bid and strconv
vet for s390x. `TestBenchBaselineShape` now also lacks
PutLongFixed/128/long.

Next in line: values of eight digits or fewer that the short path turns
down (11% of euro's formatting, 5% of banking's) still take the general
path.

## Short coefficients of a million or more in the fixed form

The values of eight digits or fewer that the short path turned down,
counted per app (same temporary counter, removed):

| app | turned down | 10^6 or more | 5 to 9 places | exponent > 0 | below 0.0001 |
|---|---|---|---|---|---|
| euro | 243 | 103 | 140 | 0 | 0 |
| banking | 100 | 100 | 0 | 0 | 0 |
| tax fed, ca, nc | 0 | | | | |

Euro's 140 with more places: 5 places 42, 6 35, 7 31, 8 26, 9 6; mostly
six digits (its rates and intermediate results). The values of 10^6 or
more have seven or eight digits at an exponent of 0 or -1.

The second group is the simpler fix and the only one for banking. The
short layout depends only on a coefficient below 10^8 and 0 to 4 places;
IsShort128's bound on the magnitude is there because 'g' writes 10^6 and
more in exponential notation. 'f' writes them plain, so:

- `bid.IsShortFixed128` and `bid.UBDIsShortFixed128`: the short test
  without the magnitude bound.
- strconv's `formatDecimal` and `appendDecimal` take a value that
  `isShortDecimal` refuses when the format is 'f' and
  `isShortFixedDecimal` accepts it; the test runs only after the short
  test fails.
- `TestShort128` checks `IsShortFixed128` on all its values and, for
  those it accepts, `PutShort128` and `ShortForm128`'s length against
  the fixed form built from the digits; `TestUBDIsShortFixed` checks the
  UBD tests against BID on the UBD pool and 200,000 random values.

Microbenchmarks ('f', -1, median of 9, bytes and allocations unchanged):
seven-digit integers of a million or more 22.79 → 12.37 ns (−45.7%);
eight digits with one place 26.66 → 12.74 (−52.2%); every other row,
'f' and 'g', within ±1.6% and not separated.

Apps at level 2, 11 rounds interleaved, outputs equal:

| app | best | median |
|---|---|---|
| euro | 759.6 → 751.3 (−1.1%) | −0.9% |
| banking | 635.2 → 632.6 (−0.4%) | −1.1% |

(Euro's times are lower than in the previous section's run, before and
after alike: the machine, not the code.)

arm64 instructions: `formatDecimal` 88 → 96, `appendDecimal` 192 → 200.

Correctness: as above, plus 2M random coefficients below 10^8 at
exponents −6 to 2 (half of a million or more) through `FormatDecimal`
and `AppendDecimal` at five amounts of spare capacity against the fixed
form, natively, under `-race` and for amd64; strconv, math/decimal, fmt,
internal/decimal tests; vet for s390x. `TestBenchBaselineShape` now
also lacks IsShortFixed/128/mixed.

Left: euro's 140 values with five places or more, which need the short
layout widened past four places (about 1% for euro, nothing elsewhere).

## Five to seven places in the fixed form

Euro's 140 short coefficients at five places or more: 5 places 42, 6
35, 7 31, 8 26, 9 6. With at most eight digits and at most seven places,
the form is at most nine bytes after the sign and every digit it shows is
among the eight eightQuads gives, so PutShort128's masks and stores hold
unchanged (`8*frac` stays inside a word, and the point never lands on
the first byte). At eight places or more a value below one shows more
than eight digits, past the word: those 32 are left on the general path.

'g' is unchanged: it writes a value whose leading digit is below 10^-4
(0.0000123) in exponential notation, so its short test would need a
lower bound on the coefficient as well. `IsShortFixed128` and
`UBDIsShortFixed128` alone go down to an exponent of -7
(`fixedMinExp`); the rotation is `fixedExp128` in encoding.go, where
`TestRoundIsTheOnlyPacker` allows the exponent field to be read.

`TestShort128` covers exponents -8 to 2, so it checks the new places
against the fixed form built from the digits, and fails if the bound is
moved to -8 (PutShort128 then writes "0.\x00\x00\x00\x00000." for
0E-8). `TestUBDIsShortFixed` now draws exponents from -10 to 2; the
end-to-end check in strconv runs exponents -9 to 2.

Microbenchmarks ('f', -1, median of 9; bytes and allocations unchanged):
one digit and five places (2.20371) 26.75 → 12.41 ns (−53.6%); 0.00
and five digits (seven places) 27.97 → 12.95 (−53.7%). The 11-digit
amount and the million-with-cents rows, which take the long path the
change does not touch, came out 5–6% faster: placement. The rest within
±1%.

Euro at level 2, 11 rounds interleaved, output equal: best 795.0 →
786.6 ns (−1.1%), median 805.9 → 790.5 (−1.9%). (The machine ran slower
this session than the last, both builds alike.) No other app formats
values with five places or more.

arm64 instructions: `formatDecimal` 96 → 100, `appendDecimal` 200 → 204.
