# UBD kernel tuning at -d=ubdlevel=1

Punch list item 1, step a: profile and tune the `ubd*` kernel entries in
`internal/bid` that the compiler calls at `-d=ubdlevel=1`, where every
decimal128 operation is a kernel call. Branch `ubd-kernel-tune`, off `ubd128`
at 447e9c0b4e. No inline-path (compiler) changes and no new kernel entries:
the kernel contract is unchanged.

## Method

`BenchmarkUBDKernel` (`src/internal/bid/ubd_kernel_bench_test.go`) times each
UBD entry against the BID entry the XOR-BID compiler calls for the same
operation (`addSlow128`, `subSlow128`, `mulSlow128`, `div128`,
`quantizeSlow128`, `quietCmpSlow128`), each on its own encoding of the same
operands, so neither side converts. Both are called through function values
(`export_test.go`), as a call site calls the kernel: never inlined, and no
adapter between. Rows are `Op/class/side` over 1024 pairs in a fixed
pseudo-random order:

| class | x | y | negative |
|---|---|---|---|
| money | 3–10 digits at 10^-2 | 3–10 at 10^-2 | 0% |
| cents10neg | same | same | 10% |
| gap2 | 3–10 at 10^-2 | 1–5 at 10^-4 | 0% |
| gap2neg | same | same | 10% |
| int | 1–6 at 10^0 | 1–6 at 10^0 | 0% |
| w19 | 17–19 at 10^-6 | 17–19 at 10^-9 | 0% |
| w25 | 22–25 at 10^-6 | 22–25 at 10^-9 | 10% |
| d34 | 34 at 10^-20 | 34 at 10^-24 | 10% |
| q34 | 30–34 at 10^-28 | 1–6 at 10^-2 | 10% |
| mixexp | 3–10 at 10^-2 | 1–8 at 10^-2 or 10^-4, at random | 10% |

`TestUBDKernelBenchAgrees` checks that both sides give the same result on
every benchmarked pair. A/B timings below are two test binaries built with
`-ldflags=-funcalign=64` and run interleaved, 6–8 runs each, medians; Apple
M3 Pro.

    cd src && GOEXPERIMENT=decimal ../bin/go test internal/bid -run '^$' -bench UBDKernel -count 6

Noise: some rows (Add and Sub on d34 in particular) are bimodal from run to
run, 8.4–8.7 or 9.3–9.5 ns, in every build including the unchanged base, so a
5–9% change on them in a single A/B is not evidence. Checked by building the
base code with padding functions that moved `ubdAddSlow128` to the offsets
seen in the new builds: all builds showed the same two modes.

## Where level-1 time goes in the applications

The five apps of go-decimal128-app-benchmark built at `-d=ubdlevel=1` and
profiled in compute mode (a scratch copy of the harness with
`runtime/pprof`; the benchmark repository is unchanged): the hot kernel
functions are word paths. tax-fed: `ubdAddWordTTE` 27% of samples,
`ubdCmpWord` 8%, `ubdEqual128`, `ubdLess128`; telco: word add, quantize word
(`ubdQuantizeWord`), `ubdMulShort128`. Two-word paths did not show. (Risk's
profile stops early because its harness reports more than once; euro and
banking are dominated by formatting and I/O.)

## Changes

### 1. Add and subtract decide word pairs in the entry (5232eddb43, 04fc1d5ba7)

`ubdAddTTE128` and `ubdSubTTE128` called `ubdAddSlow128`, which called
`ubdAddLong128` for a word pair: two calls with frames of 112 and 192 bytes,
and in `ubdAddLong128` a branch on the sign of a difference that `x - y` of
random positive amounts took about half the time. The entries now call
`ubdAddWordTTE`, a frameless leaf that adds or subtracts by mask.

A first version read the sign of a difference from the top bit of the
high word; `TestUBDAddWordAgrees` (new) and the existing pool test caught it:
a word scaled by 10^19 can pass 2^127. The sign now comes from the carry out.

### 2. Word compares call one frameless leaf (e627581a92)

A compare of word coefficients made three calls: the predicate,
`ubdQuietCmp128` and `compareWords`, which does not inline. The predicates
and `ubdQuietCmp128` now call `ubdCmpWord` first. The frame mattered more
than the calls: with a bounds check (`pow10tab64[min(g, 19)]` on a signed
`g`) giving it a frame, `ubdCmpWord` made Less slower than before, 6.0
against 4.1 ns; with the index unsigned and no frame, 3.7.

### 3. Add of one exponent skips the scaling (9d3462dbed)

| row | before | after | BID |
|---|---|---|---|
| Add money | 3.26 | 2.20 | 3.46 |
| Sub money | 3.31 | 2.21 | 6.59 |
| Add gap2 | 3.26 | 3.33 | 3.46 |
| Add mixexp | 3.39 | 3.42 | 4.76 |

The same branch in `ubdCmpWord` gained 20–27% on one exponent but took Less
and Equal on mixexp from 4.0 to 6.2 ns (a mispredict half the time) and
Compare at gap 2 up 15–20%. Set aside.

## Tried and dropped

- The add word path written into the entry instead of a leaf call: no
  change (3.26 against 3.25 ns). The path is bound by its own work, not by
  the call.

## Standing: UBD against BID after change 3

ns per call, medians (UBD / BID); word classes:

| | money | gap2 | int | w19 |
|---|---|---|---|---|
| Add | 2.20 / 3.46 | 3.33 / 3.46 | 2.20 / 3.46 | 3.32 / 3.46 |
| Sub | 2.21 / 6.59 | 3.38 / 4.85 | 2.21 / 6.63 | 3.36 / 5.05 |
| Mul | 2.01 / 3.19 | 2.01 / 3.19 | 2.01 / 3.19 | 14.3 / 13.0 |
| Div | 12.6 / 15.7 | 11.2 / 14.3 | 11.4 / 15.2 | 16.3 / 15.5 |
| Quantize | 2.00 / 3.04 | 2.49 / 3.08 | 2.00 / 3.04 | 2.48 / 3.07 |
| Less | 3.76 / 4.62 | 3.77 / 4.69 | 3.79 / 4.64 | 3.78 / 4.29 |

Still slower than BID: two-word operands, by 6–11% (Add w25 and q34, Mul
w19/w25/d34/q34, Quantize w25/d34/q34, Sub d34). The cause is structural:
the UBD entry calls `ubdAddSlow128`, `ubdMulSlow128` or
`ubdQuantizeSlow128`, two frames, where the XOR-BID compiler calls BID's
slow function directly, one frame. Removing it means writing those bodies
into the entries (duplicating them for the Rnd entries) or changing which
function the compiler calls, which is a contract change; not done.
