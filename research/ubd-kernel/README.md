# UBD kernel tuning at -d=ubdlevel=1

Punch list item 1, step a: profile and tune the `ubd*` kernel entries in
`internal/bid` that the compiler calls at `-d=ubdlevel=1`, where every
decimal128 operation is a kernel call. Branch `ubd-kernel-tune`, off `ubd128`
at 447e9c0b4e. No inline-path (compiler) changes and no new kernel entries:
the kernel contract is unchanged.

## Results in brief

The five apps of go-decimal128-app-benchmark, compute mode, best of 7
interleaved runs, this branch's kernel (d25c81928c) against 447e9c0b4e's,
Apple M3 Pro, both built with `-ldflags=-funcalign=64`. Outputs equal the
goldens (tax stderr, banking, telco's SHA-256, risk's 20 files; euro equal to
447e9c0b4e's output) at every level.

| app | level 1 | level 2 | level 3 |
|---|---|---|---|
| telco | −14.7% | −0.3% | +0.0% |
| tax fed | −12.3% | −5.5% | −0.1% |
| risk_management | −12.0% | −8.6% | −4.7% |
| tax ca | −11.1% | −6.2% | +1.0% |
| tax nc | −7.0% | −4.6% | −0.1% |
| banking | −4.2% | −3.0% | −1.1% |
| euro_conversion | −1.2% | −0.8% | −1.0% |

Level 2 gains because its inline paths hand the kernel everything they do
not take (different exponents, mixed signs); level 3's BID-derived inline
paths take most of that themselves.

Per call, against BID's kernel on the same operands (UBD before → after,
BID; ns):

| | before | after | BID |
|---|---|---|---|
| Add money | 3.71 | 1.97 | 3.46 |
| Sub money | 6.01 | 2.02 | 6.58 |
| Add cents, 10% negative | 4.86 | 2.65 | 4.45 |
| Add amount + rate (gap2) | 3.71 | 2.48 | 3.46 |
| Add 22–25 digits (w25) | 5.93 | 5.63 | 5.87 |
| running total, money (AddChain) | 4.23 | 2.39 | 3.71 |
| running balance, gap2 (SubChain) | 4.76 | 2.39 | 5.04 |
| running total, 17–19 digits (AddChain w19) | 5.51 | 2.33 | 5.32 |
| running total, 22–25 digits (AddChain w25) | 6.34 | 3.27 | 6.38 |
| Mul 14–16 digits (w16) | 4.08 | 2.01 | 3.19 |
| Mul 17–19 digits (w19) | 14.42 | 10.74 | 13.03 |
| Mul quotient × amount (q34) | 11.43 | 9.37 | 10.64 |
| Div money | 12.53 | 11.85 | 16.16 |
| Div 17–19 digits (w19) | 16.25 | 15.23 | 15.50 |
| Less money | 4.18 | 3.72 | 4.63 |
| Equal money | 6.12 | 4.04 | |
| Compare money | 5.75 | 4.49 | |
| Min money | 5.89 | 4.29 | 8.21 |
| Quantize gap2 | 2.47 | 2.17 | 3.06 |
| Quantize quotient to cents (q34) | 5.04 | 4.52 | 4.53 |
| RoundToIntegral money | 3.44 | 2.92 | 4.85 |
| RoundToIntegral q34 | 6.42 | 4.52 | 6.60 |
| int64(x), money | 4.50 | 2.38 | 5.92 |
| compound: quantize(acc × rate, 0.01) | 6.50 | 6.38 | 9.48 |

Still slower than BID (8 of 143 rows): subtract on two-word operands (Sub
q34 7.60 against 6.80, Sub w25 7.04 against 6.78, SubChain q34 +7%), whose
entry negates y and calls the add entry, two frames where BID's subSlow128
is one; Quantize w25 2.92 against 2.74; and the d34 rows (34-digit operands),
which are bimodal from run to run, and whose quantize is the invalid
operation, a NaN. Sub q34 is also the one row clearly worse than before
(6.46 → 7.60); it could not be pinned to one change.

**The structural fix** that closed most of the two-word gap: the entries
called a slow function (`ubdAddSlow128`, `ubdMulSlow128`, `ubdDiv128`,
`ubdQuantizeSlow128`), a second frame, where the XOR-BID compiler calls
BID's slow function directly. The Rnd entries have no caller in compiled
code, so the slow function's body could become the TTE entry's, with `const
mode = ToNearestEven` at its top so its mode tests fold, and the Rnd entry
calls the TTE entry for ToNearestEven and the `Long` function, which takes
every mode, otherwise. No code is duplicated. Quantize and round to integral
call a leaf (`ubdQuantizeWideTTE`) instead.

## Method

`BenchmarkUBDKernel` (`src/internal/bid/ubd_kernel_bench_test.go`) times each
UBD entry against the BID entry the XOR-BID compiler calls for the same
operation (`addSlow128`, `subSlow128`, `mulSlow128`, `div128`,
`quantizeSlow128`, `roundToIntegral128`, `quietCmpSlow128`, and `Min128`,
`Int64From128` for min and conversion), each on its own encoding of the same
operands, so neither side converts. Both are called through function values
(`export_test.go`), as a call site calls the kernel: never inlined, no
adapter between. Rows are `Op/class/side` over 1024 pairs in a fixed
pseudo-random order:

| class | x | y | negative |
|---|---|---|---|
| money | 3–10 digits at 10^-2 | 3–10 at 10^-2 | 0% |
| cents10neg | same | same | 10% |
| gap2 | 3–10 at 10^-2 | 1–5 at 10^-4 | 0% |
| gap2neg | same | same | 10% |
| int | 1–6 at 10^0 | 1–6 at 10^0 | 0% |
| w16 | 14–16 at 10^-4 | 14–16 at 10^-6 | 10% |
| w19 | 17–19 at 10^-6 | 17–19 at 10^-9 | 0% |
| w25 | 22–25 at 10^-6 | 22–25 at 10^-9 | 10% |
| d34 | 34 at 10^-20 | 34 at 10^-24 | 10% |
| q34 | 30–34 at 10^-28 | 1–6 at 10^-2 | 10% |
| mixexp | 3–10 at 10^-2 | 1–8 at 10^-2 or 10^-4, at random | 10% |

Most rows time throughput: the calls are independent. `AddChain` and
`SubChain` are a running total and a running balance, each call waiting for
the one before, and `BenchmarkUBDKernelCompound` is compounding,
`acc = quantize(acc × rate, 0.01)`. **Both kinds are needed**: the first
word-path version won every throughput row and lost every chain row to BID
by 13–45% (below).

`TestUBDKernelBenchAgrees` checks that both sides agree on every benchmarked
pair; `TestUBDAddWordAgrees`, `TestUBDCmpWordAgrees` and
`TestUBDInt64WordAgrees` check the new word paths against the BID kernel on
two million random pairs each. `-bid.vectors=ubd` (new) runs the published
vectors (GDAS decTest, Intel readtest) through the UBD entries: 70705 pass, 0
fail, the kernel target's count. Before it, no published vector reached a UBD
entry.

Correctness on other targets: internal/bid's tests, the `ubd` vector run,
math/decimal, strconv and internal/decimal also pass as amd64 under Rosetta
(which takes the native 128-by-64 divide in the divide path). No 32-bit
target can run here; the package and its test binary build for 386 and
arm. Reviewing the unsigned-index changes for 32-bit targets found one bug,
fixed in 3f4c7e279d: `ubdCmpWord`'s test for a swamped y wrapped at 32 bits.
**A 32-bit test run is still owed.**

A/B timings: two test binaries built with `-ldflags=-funcalign=64`, run
interleaved 6–8 times, medians.

    cd src && GOEXPERIMENT=decimal ../bin/go test internal/bid -run '^$' -bench UBDKernel -count 6
    BID_DECTEST_DIR=... BID_INTEL_READTEST=... GOEXPERIMENT=decimal ../bin/go test internal/bid \
        -run 'TestPublishedVectors$' -args -bid.vectors=ubd

The apps were built from a scratch copy of the benchmark repository whose
harness writes a CPU profile when `D128_CPUPROFILE` is set; the repository
itself is unchanged.

Noise: Add and Sub on d34 are bimodal from run to run (8.4–8.7 or 9.3–9.5
ns) in every build, the base included, so a 5–9% change there in one A/B is
not evidence. Checked by padding the base code so that `ubdAddSlow128` moved
to the offsets of the new builds: all builds showed the same two modes.

## What the time went to

**Frames.** A function with any call, a panic's included, gets a prologue
(stack check, link register saved) and an epilogue. On the word paths these
were most of the cost. `ubdCmpWord` with a bounds check in it (a signed index
into `pow10tab64`) made Less slower than three calls had been, 6.0 ns against
4.1; with the index unsigned and bounded by `min`, no frame, 3.7.
`ubdQuantizeWord` had a frame for a bounds check and a division-by-zero
check. A leaf with no frame, called from the entry, costs little; a helper
with a frame in between costs a lot (the round-to-integral entries through
`ubdRoundToIntegralWord`: 3.71 ns against 3.45 calling the leaf directly).

**The chain.** Masks instead of branches won independent operations with
mixed signs and lost every chain: a sum chosen by masks put a dozen
dependent instructions between one total and the next where BID's predicted
branches put three. Where a branch's direction goes with the kind of data (one
exponent and one sign; which operand has the larger exponent), it stays a
branch; where it goes either way for random data (the sign of a difference),
a select. This is the same lesson as the reverted sign-agnostic inline add
(04befab964).

**The level-2 traffic.** At level 2 the kernel sees what the inline paths
refuse: different exponents and mixed signs. The masked two-exponent path
made level 2's tax fed 3.5% slower than 447e9c0b4e (bisected to the first
word-path commit); with branches it is 4.4% faster.

## Changes

Names are as they were at each commit; `ubdAddSlow128`, `ubdMulSlow128`
and `ubdDiv128` have since become the TTE entries' bodies (change 11).

1. **Add and subtract decide word pairs in the entry** (dbf30ae565,
   04fc1d5ba7). The entries called `ubdAddSlow128`, which called
   `ubdAddLong128`: two frames of 112 and 192 bytes. They now call
   `ubdAddWordTTE`, a leaf. A first version read the sign of a difference
   from the top bit of the high word; the tests caught it (a word scaled by
   10^19 can pass 2^127).
2. **Word compares call one leaf** (e627581a92): the predicates,
   `ubdQuietCmp128`, and later min and max, call `ubdCmpWord` for a word pair
   instead of `ubdQuietCmp128` → `compareWords`.
3. **Add of one exponent skips the scaling** (9d3462dbed).
4. **Multiply takes every word product below 2^112 inline** (92e023c89d):
   `ubdMulShort128`'s bound was BID's 2^96; UBD's high word holds a
   product's top 48 bits below the sign. `ubdMulSlow128` no longer sends word
   pairs to `ubdMulLong128`: those that reach it mostly round, and its own
   two-word path rounds them in one call.
5. **int64 conversion and min/max take word operands directly**
   (18eabce336). A sign chosen by mask in the conversion was slower than the
   branch (3.46 ns against 2.33 for positive values) and was set aside.
6. **The quantize word path is a leaf** (b77418666c); the round-to-integral
   entries call it directly.
7. **cDigits128 and cCmpFinite128 are leaves** (e6136cb4b1), shared with
   the BID kernel.
8. **Running totals** (305cbb333e): one exponent and one sign is a branch to
   an add and its carry; one exponent and opposite signs selects the
   difference that did not borrow; `ubdAddSlow128` sums two-word coefficients
   of one exponent and one sign before its general path.
9. **Two exponents branch as BID's do** (4b71df59ee).
10. **Quantize of two-word coefficients is one call** (cc28c29fff):
    `ubdQuantizeWideTTE`, `ubdQuantizeSlow128`'s paths for ties to even.
11. **Multiply, add and divide past their fast paths are one call**
    (924e2adb91, f038ed971f, e62da7db33): the slow function's body is the
    TTE entry's (above).
12. **Round to integral of two-word values is one call** (d25c81928c):
    `ubdQuantizeWideTTE` against 1E0.
13. **uint64 conversion takes word operands directly** (6cd480a681), as
    int64 does: Uint64 money 3.98 → 2.45 ns (BID 5.64).

The benchmark also times the Rnd quantize entry toward zero (806a45c26f),
which telco's truncated taxes call (9% of its level-1 profile): word rows
1.98–2.39 ns against BID's 3.02–3.06.

## Tried and dropped

- Equal without its identical-encoding test: word rows 40% slower (4.1 →
  5.9 ns), though the test only answers for equal values. Kept.
- The add word path handing the operands it refuses back in its results,
  so that the entry need not save them across the call: the prologue's
  stores went, but word adds at two exponents and of mixed sign were 7–10%
  slower.
- `ubdCompare128` (the 3-way compare entry) calling `ubdCmpWord` for a
  word pair itself instead of through `ubdQuietCmp128`: Compare 20–37%
  slower, and min and max, which called it inline, 13–19% slower on
  two-word operands once it no longer inlined (cost 144).
- The predicates calling `cCmpFinite128` themselves for two finite
  two-word operands, through a helper meant to inline: it did not (cost
  100), and two-word compares were 17–44% slower.

- Quantize's two-word rounding alone in a leaf: q34 4.25 ns, but every other
  two-word quantize paid the call before falling through (w25, d34 +33–40%);
  testing the exponents in the entry first made the word rows 11% slower
  instead (the entries' layout moves their word paths by up to ±10%).
- The add entry's body folded in with the word-pair test kept in both
  places: the entry saved and retested it around the word path's call, and
  word adds at two exponents were 5% slower.

- The add word path written into the entry instead of a leaf call: no
  change in throughput.
- The one-exponent branch in `ubdCmpWord`: 20–27% faster on one exponent,
  but Less and Equal on mixexp 50% slower (a mispredict half the time), and
  Compare at gap 2 15–20% slower.
- A two-word one-exponent sum in the add entries: as a helper over the
  inlining budget it was a call on every two-word add (+9–23%); with the test
  in the entry, the word rows lost 7–33%. In `ubdAddSlow128` it costs the word
  rows nothing.
- `ubdMulShort128` widened by comparing with MaxCoeff128: over the inlining
  budget (83 against 80), a call on every multiply.
- `divPow10Recip128` without its bounds checks: no change on q34 quantize,
  and a bad index would no longer panic; reverted.
- A reciprocal multiply in place of the divide in `ubdQuantizeWord`, for
  compounding's chain: not tried after reading the existing measurement (3%
  slower) and counting the chain (one or two cycles at most).

## Not done

- Subtract on two-word operands: one call would need a copy of the add
  entry's body.
- Divide: four dependent divides by a divisor below 2^32 are most of its
  10–12 ns on word operands; BID's same code takes 14.7–16.2.
- Formatting (`PutShort128`, `writeWordDigits`, `PutLong128`) is 6–8% of
  the tax and telco profiles at every level and has frames for bounds
  checks; it is not a compiler-called entry, so outside this step.

## What level 2 sends the kernel (2026-11-23)

Counted by pointing the compiler's kernel symbols at counting wrappers
(a scratch toolchain; only calls the compiler emits are counted), each
app run once at level 2. Calls by operand class:

| app | dominant kernel traffic at level 2 |
|---|---|
| telco | quantize only: 2.0M ties-to-even and 3.0M toward-zero, every one a one-word x made coarser by 2, 3 or 4 digits; 6 adds |
| risk | add/sub same exponent, mixed sign (1.9k + 2.5k); add/sub exponents 1–19 apart (2.7k); div word (2.6k); quantize coarser by 4 (4.8k) and two-word x (2.4k) |
| banking | add/sub exponents 1–19 apart (9.7k) and two-word (4.1k); div (7k, 60% two-word); `<` exponents apart (3.5k); round-to-integral (≈10k, 60% two-word fraction) |
| euro | add/sub/`<` exponents apart; div; round-to-integral of fractions |
| tax fed, ca, nc | tens of calls: add/sub/`<` exponents apart |

Pairs with exponents more than 19 apart: none. Quantize already at the
target exponent: 22 of risk's 12k, none elsewhere. Round-to-integral of
an integer (exponent ≥ 0): 3–7% of those calls. int64(x): every call an
integer at exponent 0, at most 200 a run.

### The trivial first paths at the call site (b24b53112a)

Level 2 tests three kernel first paths before the call: int64/uint64 of
an integer at exponent 0, round-to-integral of a finite x at exponent
≥ 0, and quantize of a one-word x already at y's exponent. Level 1 and
level 2 agree on every pair of 43 edge values. Apps at level 2, best of
7 against ubd-kernel-tune: all within ±1.6% (telco +1.6%, risk +1.4%,
the rest ±0.5%; medians −1.6% to +1.8%): no measurable effect, as the
counts predict. Kept at the user's direction for quantize.

### Specialized level-2 entries, not made

The inline paths settle same-exponent, same-sign word pairs, so a level-2
add entry could skip the leaf's first test. Upper bound, measured by
deleting that test: −2% on single different-exponent word adds and
subtracts (about 0.05 ns), which is below what the apps can show.
`ubdCmpWord` is branch-free, so there is no test to remove there; Equal's
identical-encoding test is redundant at level 2, but Equal is at most 40
calls a run in these apps.

### Quantize to a coarser exponent inline at level 2 (8032567fef)

Quantize at level 2 now runs level 3's inline word paths, less the one
that scales x up: x at y's exponent is x, and a one-word x 1 to 19 places
finer than y is divided by the power of ten and rounded in the mode
(constant modes fold). Level 1 and level 2 agree on 3M random quantizes
in every mode and on the 43 edge values; `QuantizeCent128`'s codegen
test now expects the divide. Apps at level 2, best of 7 against
ubd-kernel-tune (medians alike):

| app | change |
|---|---|
| telco | −9.2% |
| risk | −5.1% |
| tax fed | −0.6% |
| tax ca | −0.4% |
| euro | −0.2% |
| tax nc | +0.4% |
| banking | +0.7% |

Telco and risk are the apps whose level-2 kernel traffic is quantize
(above); the others make few quantize calls.

### Call-site size (research/ubd-size's probe)

Each probe function is one `//go:noinline` operation, so its size is the
call site's code plus a frame; nothing else is live, so no spills are
counted (a lower bound). Added for this measurement: `QuantizeCent`
(y the constant 0.01), `QuantizeTZ` (`ToZero`), `QuantizeVar` (the mode
a variable), `Trunc`, `RoundToEven`, `int64(x)`, `uint64(x)`. Bytes
(instructions); level 1 and level 2 before are identical:

| op | arch | level 2 before | level 2 after | change | level 3 |
|---|---|---|---|---|---|
| Quantize | amd64 | 68 (18) | 231 (61) | +163 | 326 (86) |
| Quantize to 0.01 | amd64 | 66 (17) | 205 (52) | +139 | 308 (75) |
| QuantizeMode ToZero | amd64 | 74 (20) | 179 (47) | +105 | 286 (74) |
| QuantizeMode, variable mode | amd64 | 79 (21) | 357 (91) | +278 | 437 (112) |
| Round, Trunc, RoundToEven | amd64 | 49–53 (15) | 66–68 (18) | +15–17 | 124–180 |
| int64(x) | amd64 | 49 (15) | 98 (30) | +49 | 98 (30) |
| uint64(x) | amd64 | 49 (15) | 53 (16) | +4 | 53 (16) |
| Quantize | arm64 | 68 (17) | 184 (46) | +116 | 240 (60) |
| Quantize to 0.01 | arm64 | 68 (17) | 172 (43) | +104 | 224 (56) |
| QuantizeMode ToZero | arm64 | 76 (19) | 152 (38) | +76 | 208 (52) |
| QuantizeMode, variable mode | arm64 | 80 (20) | 292 (73) | +212 | 344 (86) |
| Round, Trunc, RoundToEven | arm64 | 60–64 (15–16) | 72–76 (18–19) | +12 | 116–152 |
| int64(x) | arm64 | 60 (15) | 96 (24) | +36 | 96 (24) |
| uint64(x) | arm64 | 60 (15) | 64 (16) | +4 | 64 (16) |

A quantize with a constant mode grows by 76–163 bytes, about 2.5× the
call alone, and stays 50–100 bytes under level 3's; a variable mode
keeps all six roundings and costs 212–278. The trivial exits cost 4–17
bytes, `int64(x)` 36–49 (its sign application).
