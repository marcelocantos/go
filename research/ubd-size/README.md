# UBD decimal128: inline code size vs XOR-BID

Experiment on Marcelo Cantos's Go decimal fork
([marcelocantos/go](https://github.com/marcelocantos/go), branch `decimal`),
pinned at `5032fd77`, the commit his proposal repository builds
([go-decimal-proposal](https://github.com/marcelocantos/go-decimal-proposal),
`toolchain.lock`).

- **Baseline (XOR-BID):** `5032fd77` unchanged. decimal128 is stored as its
  IEEE BID encoding with the high word XORed with `0x3040000000000000` (the
  encoding of +0E+0), so that all bits clear is the zero value. The compiler
  inlines integer fast paths for the operators and calls the kernel for the
  rest.
- **UBD (branch `ubd128`):** decimal128 is stored in the UBD layout
  ([whitepaper](https://migueldecimal128.github.io/decimal128-www/whitepapers/ubd.html)):
  qExp in bits 127..114 (two's complement, unbiased), sign in bit 113,
  binary coefficient in bits 112..0. All bits clear is +0E+0 with no mask.
  decimal64 stays XOR-BID in both builds.

The goal is generated code size per call site, not speed. Nothing here has
been benchmarked.

## Where the UBD build stands

By default every decimal128 operation compiles to its kernel call, with
decimal128-go's add fast paths in front of `+` and `-` and its multiply
fast path in front of `*`:

- **`x + y`:** decimal128-go's first-level path (`d128_add_rnd_ctxnull`),
  narrowed to one-word coefficients: one guard,
  `((xh ^ yh) &^ sign) | xh<<15 == 0` (equal qExp, and no coefficient bits
  in the high word, which also excludes the specials), then a branch on the
  signs: `ADD; ADC $0` with x's qExp and sign riding along in its high word,
  or, for opposite signs, `-`'s effective subtraction (since 2026-11-22;
  before, opposite signs took the call). Every other pair,
  including coefficients of 2^64 or more, calls `ubdAddTTE128`. (The
  two-word path it replaced, old level 9, took coefficients up to 2^112
  with a second test; deleted.)
- **`x - y`:** decimal128-go's second-level path (the first test of
  `d128_add_slow_rnd_ctxnull`), for operands of one sign and one qExp, which
  are an effective subtraction, narrowed to one-word coefficients: `+`'s
  guard, one borrow, the difference negated by the borrow's mask with the
  sign flipped, an exact zero +0. Every other pair calls `ubdSubTTE128`.
  (The path it replaced, old level 9, took coefficients up to 2^112 with a
  two-word borrow chain; deleted.)
- **`x * y`:** decimal128-go's first-level multiply path, narrowed: both
  coefficients below 2^56 (the high words' coefficient bits clear and the low
  words below 2^56, one OR'd test) and the qExp sum in range (one unsigned
  compare). The product is then below 2^112 and needs no check after the
  64×64 multiply; the header is the sign XOR and the qExp sum. Every other
  pair calls `ubdMulTTE128`. Taking full 64-bit coefficients would cost a
  test of the product's high word after the multiply: 2 more instructions on
  amd64 (30), 1 on arm64 (21).
- **`x == 0` and `x != 0`** (against a constant zero the compiler can see,
  at any quantum): `(hi<<15)|lo == 0`, with no call. A well-formed finite
  coefficient is at most 10^34 − 1, so the value is zero exactly when its
  113 coefficient bits are clear, and every special has the `0x1F` prefix in
  its coefficient bits, so none passes. amd64 is `SHLQ; ORQ; TESTQ`, arm64
  `ORR R1<<15; CMP`.
- **`x < 0`, `x <= 0`, `x > 0`, `x >= 0`** (against a constant zero):
  inline, with no call. `v = (hi<<14) | (lo != 0)` puts the sign in the top
  bit over the coefficient, so v is negative exactly when the sign is set
  and 0 exactly at +0 (a canonical infinity has a nonzero coefficient, its
  prefix). Then `x > 0` is `int64(v) > 0`, `x <= 0` is `int64(v) <= 0`, and
  `x < 0` and `x >= 0` are the same tests on `-v` (−0 gives the least int64,
  which negation keeps). A NaN, found by its tag `hi>>59 == 15`, branches to
  false.
- **`x == y`, `x != y`, `x < y`, `x <= y`, `x > y`, `x >= y`:** `+`'s
  guard (one qExp, one sign, one-word coefficients, so no NaN tag test),
  then `xl == yl`, or the low words XORed with the sign mask and compared
  once. The two-word path it replaced (old level 9, deleted) was
  decimal128-go's first-level guard, one qExp and
  one sign (`(xh ^ yh) >> 49 == 0`), and not NaN (`xh>>59 != 15`, since NaNs
  of one sign share a qExp field), then inline. With the top 15 bits equal, the
  128-bit words compare as the coefficients do: a canonical value equals
  another exactly when the bits are (`(xh^yh)|(xl^yl) == 0`), and the
  orderings are one borrow chain on the words XORed with the sign mask,
  which reverses unsigned order for a negative sign. Every other pair calls
  the predicate entry for the operator (`ubdLess128` and the rest), which
  returns the result as a bool; `x > y` and `x >= y` have their own entries,
  so the operands are passed where they are, not swapped.
- **Everything else** (`/`,
  `decimal.Quantize`, `decimal.Round` and the other integral roundings) is
  the kernel call alone.

`-d=ubdlevel=N` selects how much is inlined. The baseline every level is
measured against is Marcelo's XOR-BID toolchain (`5032fd77`), a different
storage format that no level reproduces.

| Level | Inline fast paths |
|---|---|
| 1 | none: every operation is its kernel call (comparisons call the predicate entries); the smallest code |
| **2** (default, `0`) | the narrowed paths above: `+` and `-` with one-word coefficients, `*` with coefficients below 2^56, comparisons with zero with no call, comparisons of two values with one-word coefficients |
| 3 | the inline paths derived from the BID lowering (see History), the same kernel calls behind them; the most code |

Any other value is a usage error. At level 2, `5.00 + 3.00` and
`5.00 - 3.00` are inline; `5.00 + -3.00` and `5.00 - -3.00`, operands of
opposite signs, call the kernel.

Until 2026-11-22 there were levels 1 to 10 and −1, adding paths one at a
time. The tables below and the History keep the old numbers; they map as:

| Old level | Now |
|---|---|
| 1 (kernel calls; comparisons called `ubdCompare128`) | 1 (comparisons call the predicate entries, as old 8 did) |
| 2–9 (two-word add and comparison paths, added in steps) | removed; the two-word paths were deleted |
| 10 (narrowed paths, the default) | 2, the default |
| −1 (BID-derived paths) | 3 |

## The UBD kernel contract

The compiler calls only these entries of `internal/bid` for decimal128
(`internal/bid/ubd.go`, "The UBD kernel contract"):

| Operation | Ties to even (no mode) | With a rounding direction |
|---|---|---|
| add | `ubdAddTTE128(x, y)` | `ubdAddRnd128(x, y, mode)` |
| subtract | `ubdSubTTE128` | `ubdSubRnd128` |
| multiply | `ubdMulTTE128` | `ubdMulRnd128` |
| divide | `ubdDivTTE128` | `ubdDivRnd128` |
| quantize | `ubdQuantizeTTE128(x, y)` | `ubdQuantizeRnd128(x, y, mode)` |
| round to integral | `ubdRoundToIntegralTTE128(x)` | `ubdRoundToIntegralRnd128(x, mode)` |
| compare | `ubdCompare128(x, y)` → (c, ok) | |
| compare, by operator | `ubdEqual128`, `ubdLess128`, `ubdLessEqual128`, `ubdGreater128`, `ubdGreaterEqual128` (x, y) → bool | |

- Each decimal128 is passed as `UBD128{Lo, Hi}`, **low word first**: the
  order in which it is laid out and passed in registers, so a call site moves
  nothing into place and the result arrives where it is returned.
- The operators round ties to even and call the TTE entries, so a call site
  sets no mode register. Quantize and integral rounding call TTE when the mode
  is the constant ToNearestEven (`decimal.Quantize`, `decimal.RoundToEven`)
  and Rnd otherwise. The Rnd entries of add, subtract, multiply and divide have
  no caller yet.
- The result of each entry equals what the exported BID function returns
  for the operands' BID encodings, read as UBD, and it is canonical UBD.
- The entries work on UBD itself: `ubd_add.go`, `ubd_mul.go`, `ubd_div.go`,
  `ubd_quant.go` and `ubd_cmp.go` are the BID kernel's general paths with
  UBD's decoding and packing, and nothing converts. BID conversions remain
  only at the interchange boundary (`internal/decimal/encoding.go`) and in
  the compiler's constant zero test.

### Well-formed UBD

UBD's 14-bit qExp field holds −8192..8191, of which decimal128 uses
−6176..6111 and the special tag 7168..8191. The 3072 values between have no
meaning. Well-formed UBD is canonical UBD, exactly the patterns the
conversion from BID produces; every other pattern is malformed, among them
the special tag without the 0x1F coefficient prefix, a finite coefficient
above 10^34 − 1, a NaN payload of 10^33 or more, and an infinity or NaN
with bits set that its encoding does not use. (BID
has no such gap: its exponent range fills its three non-`11` prefixes
exactly.) UBD is internal and never read from an external source, so a
malformed pattern can only come from a bug or from unsafe code:

- `UBDWellFormed128` states the rule; the contract covers well-formed patterns
  only, and the inline paths do not check for malformed ones.
- `math/decimal.Native128frombits`, the one way to write arbitrary words into
  a decimal128 without unsafe, stores a malformed pattern as a positive quiet
  NaN. Its comment says it should be removed.
- The UBD tests fail at once on a malformed operand or result.

BID and DPD have non-canonical encodings, which IEEE 754 reads as a zero
coefficient or a zero NaN payload; the conversion to UBD applies that
reading, so UBD has no non-canonical encodings, and a finite value is zero
exactly when its coefficient is 0. The `x == 0` test
relies on this. Every UBD the compiled code makes is canonical: the kernel's
results are, the add fast path's sum is below 2^112, the multiply path's
product below 2^112, and the subtract path's difference no larger than an
operand, all below 10^34.

The ordered comparisons with zero are complete inline: no operand needs
the kernel, so the call is gone, not merely put behind a fast path. That
saves more than the instruction counts show: Go's internal ABI has no
callee-saved registers, so a call spills every value live across it, and in
a function that makes no other call it forces a frame. At the call site
they are about the size of the call they replace: controlling a branch,
`if x > 0` is 11 instructions on amd64 and 8 on arm64, against 10 and 9 for
the call to `ubdCompare128`; as a bool value, as the table below counts it, 13 and 10 against 9 and 8.
Two smaller forms were tried and dropped: `lo != 0` as the borrow of
`0 - lo` (one more instruction on amd64, two on arm64), and the NaN test
ANDed in instead of branched (three more where the comparison controls a
branch).

## What changed in `ubd128`

- `internal/bid/ubd.go`: UBD constants, `UBDFromBID128` / `BIDFromUBD128`,
  `UBDWellFormed128`, the kernel contract and its entries.
- `internal/bid/native.go`: decimal128's native form is UBD; its zero mask
  is 0.
- `cmd/compile/internal/ssagen/bid.go`, `dpd.go`, `bid_round.go`,
  `intrinsics.go`: the UBD lowering (`ubdLevel`, `bidCallOnly`, `ubdAddNarrow`)
  and the kernel calls; `math/decimal.Quantize[shape]` is recognized with a
  constant mode, as `Round` and its relatives already were. The BID-derived
  inline paths in `bid.go`, `bid_compare.go` and `bid_round.go` use UBD bit
  positions and remain for level 3.
- `cmd/compile/internal/base/debug.go`: `-d=ubdlevel`.
- `cmd/compile/internal/ssacompile/softdecimal.go`: negation flips bit 49.
- `cmd/compile/internal/ir/decimal.go`: constants are encoded as UBD.
- `internal/decimal/arithmetic.go`: `IsNaN`, `IsInf`, `Signbit`, `Abs`,
  `Copysign` read UBD's decimal128 word.
- `math/decimal/encoding.go`: `Native128frombits` stores malformed patterns
  as NaN; `Native128bits` notes that `NativeRepresentation` does not
  describe UBD.
- Tests: `internal/bid/ubd_test.go` (`TestUBDConversion`, `TestUBDKernel`,
  `TestUBDWellFormed`), `internal/bid/operators_ubd_test.go`
  (`TestOperatorsRawUBD`: the compiled operators on raw well-formed UBD
  memory, including patterns `BID128frombits` never produces, against the
  kernel), `test/codegen/decimal.go` (the default lowering).

## Method

`probe/ops.go` has one `//go:noinline` function per decimal128 operation.
`seg.py` splits each compiled function into:

1. frame overhead (stack check, prologue, epilogue, stack-growth stub),
   which exists only because of the wrapper function;
2. inline fast paths: each code region that ends in a jump to the result;
3. slow-call setup: argument moves and the calls into the kernel.

**Inline code per call site = (2) + (3).** It excludes the frame and the
shared kernel. `scale/s.go` cross-checks it: functions with 1, 2 and 4
additions give the code added per extra `+`.

Run:

```
./measure.sh BASE_GOROOT UBD_GOROOT
UBD_GCFLAGS=-d=ubdlevel=1 ./measure.sh BASE_GOROOT UBD_GOROOT
UBD_GCFLAGS=-d=ubdlevel=3 ./measure.sh BASE_GOROOT UBD_GOROOT
```

where each GOROOT is a toolchain built with `src/make.bash` (bootstrap: Go
1.26 or later) from `5032fd77` and from `ubd128` respectively. It builds the
probes for amd64 and arm64 with `GOEXPERIMENT=decimal` and prints the tables;
`UBD_GCFLAGS` is passed to the UBD builds. The tables below were measured on
darwin/arm64 with `GOOS=linux`.

## Results (2026-11-20)

### Inline code per call site, default (level 2, old 10; measured 2026-11-21)

| Op | amd64 instr | amd64 bytes | arm64 instr | arm64 bytes |
|---|---|---|---|---|
| `x + y` | 169 → 12 (−93%) | 714 → 37 (−95%) | 119 → 8 (−93%) | 476 → 32 (−93%) |
| `x - y` | 179 → 25 (−86%) | 773 → 87 (−89%) | 126 → 14 (−89%) | 504 → 56 (−89%) |
| `x * y` | 84 → 28 (−67%) | 369 → 103 (−72%) | 65 → 20 (−69%) | 260 → 80 (−69%) |
| `x / y` | 17 → 2 (−88%) | 64 → 6 (−91%) | 15 → 3 (−80%) | 60 → 12 (−80%) |
| `x == y` | 169 → 12 (−93%) | 708 → 36 (−95%) | 131 → 7 (−95%) | 524 → 28 (−95%) |
| `x < y` | 201 → 17 (−92%) | 795 → 55 (−93%) | 152 → 10 (−93%) | 608 → 40 (−93%) |
| `x == 0` | 30 → 5 (−83%) | 135 → 14 (−90%) | 31 → 4 (−87%) | 124 → 16 (−87%) |
| `x > 0` | 41 → 13 (−68%) | 186 → 39 (−79%) | 40 → 10 (−75%) | 160 → 40 (−75%) |
| `Quantize` | 119 → 2 (−98%) | 491 → 6 (−99%) | 81 → 3 (−96%) | 324 → 12 (−96%) |
| `Round` | 58 → 3 (−95%) | 248 → 11 (−96%) | 42 → 4 (−90%) | 168 → 16 (−90%) |

XOR-BID → UBD. Per extra `+` in a longer expression (`scale/`): amd64 162 →
16 instructions (695 → 67 bytes), arm64 109 → 13 (437 → 52). With old level
9's two-word paths (deleted) the four narrowed rows were `+` 17 / 12, `-` 41 / 26, `==` 18 / 12 and `<` 21 /
16 instructions (amd64 / arm64), and the extra `+` 23 / 17.

A call site of `x / y` on amd64 is the 5-byte `CALL ubdDivTTE128` and a
1-byte `NOP`; the operands are already in the registers the entry takes.
`x == y` is the call and the test of its result.

### `x + y` and `x - y` by level (old numbering)

| Old level | `+` amd64 | `-` amd64 | `+` arm64 | `-` arm64 | per extra `+`, amd64 |
|---|---|---|---|---|---|
| 1 (now 1): call alone | 2 (6 B) | 2 (6 B) | 3 | 3 | 5 (29 B) |
| 2: first level | 17 (60 B) | 21 (77 B) | 12 | 14 | 23 (96 B) |
| 3: both levels | 52 (190 B) | 55 (205 B) | 33 | 35 | 57 (227 B) |
| 4–9 | 17 (60 B) | 41 (150 B) | 12 | 26 | 23 (96 B) |
| **10 (now 2, the default)**: narrowed to one-word coefficients | **12 (37 B)** | **25 (87 B)** | **8** | **14** | **16 (67 B)** |
| −1 (now 3): BID-derived | 101 (383 B) | 102 (388 B) | 74 | 76 | 111 (446 B) |

Old level 2's first-level path on amd64 (two-word, deleted):

```
MOVQ $0x1ffffffffffff, DX ; ANDQ DI, DX     ; yc, the one constant
MOVQ BX, SI ; SUBQ DI, SI                   ; xh - yh
MOVQ BX, R8 ; XORQ DI, R8 ; SHRQ $49, R8    ; qExp and sign differ?
LEAQ 0(SI)(DX*2), SI                        ; + 2*yc = xc + yc
TESTQ R8, R8 ; JNE slow
SHRQ $48, SI ; TESTQ SI, SI ; JNE slow
ADDQ CX, AX ; ADCQ DX, BX ; JMP done
slow: CALL internal/bid.ubdAddTTE128
```

The guard's xc + yc is (xh − yh) + 2·yc once the top 15 bits are equal, so
the 49-bit mask is loaded once: the register allocator issues a constant
again at each use (`ssacompile/regalloc.go`), a 10-byte load on amd64.

### Notes

- **arm64 rule.** The ordered comparisons' borrow chain ends in a borrow
  tested as a bool. arm64 lowered that to `NGC; CMN; CSET NE`, the borrow
  made an integer and compared again. Two rules in `ARM64.rules`
  (`(NotEqual (CMNconst [0] (NGCzerocarry f))) => (LessThanU f)` and its
  `Equal`/`GreaterEqualU` twin) read the flags instead: `CSET LO`, 2
  instructions fewer per ordered comparison. They apply to all arm64 code;
  `math/big`, `math/bits`, bigmod, the SSA tests and `test/codegen` pass.
- **`if` form.** Where a comparison with a fast path controls a branch, the
  fast path's bool and the call's merge and are tested again (`MOVL; TESTL;
  JE` on amd64). Removing that is a control-flow change (jump threading
  through the merge), not a rule; not done.
- **Frames.** In a function that makes no other call, such as the probe's
  `Eq`, the kernel call also forces a stack frame (about 12 instructions
  on amd64) that inline code did not need. The tables exclude it; in a
  function that already calls something it costs nothing more.
- **Kernel entries.** When the entries still converted through BID, they
  and the two conversions were about 2.1 KB on amd64 (2.0 KB on arm64) in a
  program that used every operation, once per program. The native kernel
  has not been sized here.
- **Speed.** Not measured here. Every operation without a fast path pays
  for a call into the kernel, which works on UBD itself. Whole-application
  timings against XOR-BID are in `go-app-benchmark`
  (`results/RESULTS.md`).

### History

The UBD build reached the default in steps, each measured with this harness
(amd64 instructions per call site; levels in the old numbering):

| Step | `+` | `-` | `*` | `==` | `<` | `== 0` | Quantize |
|---|---|---|---|---|---|---|---|
| XOR-BID baseline | 169 | 179 | 84 | 169 | 201 | 30 | 119 |
| First UBD commit (`51ad7830`): BID paths at UBD bit positions | 134 | 136 | 57 | 108 | 124 | 22 | 97 |
| Kernel words low first, one call per op (`9e964f91`) | 99 | 103 | 38 | 105 | 121 | 22 | 69 |
| Comparisons make one fallback call (`e36d759e`) | 99 | 103 | 38 | 69 | 86 | 22 | 69 |
| Zero test as one borrow chain (`e41b206a`) | 99 | 103 | 38 | 69 | 86 | 15 | 69 |
| decimal128-go's first-level add path (`2dbe5a39`) | 102 | 103 | 38 | 69 | 86 | 15 | 69 |
| TTE entries: operators pass no mode (`0a4de20d`) | 101 | 102 | 38 | 69 | 86 | 15 | 67 |
| Level 4 default (`c87f1672`) | 17 | 41 | 2 | 5 | 5 | 7 | 2 |
| First-level multiply, full 64-bit coefficients (`efdd84e4`, level 5) | 17 | 41 | 30 | 5 | 5 | 7 | 2 |
| Multiply narrowed to 2^56 (`b5c2edb4`); level 5 default (`fbb5804e`) | 17 | 41 | 28 | 5 | 5 | 7 | 2 |
| Canonical coefficients only; `x == 0` inline; level 6 default (`02d989ec`) | 17 | 41 | 28 | 5 | 5 | 5 | 2 |
| Ordered comparisons with zero inline (`x > 0`: 9 → 13, no call); level 7 default (`51b3168b`) | 17 | 41 | 28 | 5 | 5 | 5 | 2 |
| Predicate entries (level 8, not a default) | 17 | 41 | 28 | 2 | 2 | 5 | 2 |
| Comparisons inline for one qExp and one sign; level 9 default (`32e19bc3`) | 17 | 41 | 28 | 18 | 21 | 5 | 2 |
| arm64 rule: a borrow read from the flags (arm64 `<`: 18 → 16) | 17 | 41 | 28 | 18 | 21 | 5 | 2 |
| Add, subtract and comparisons narrowed to one-word coefficients; level 10 default (see `../money-form`) | 12 | 25 | 28 | 12 | 17 | 5 | 2 |
| Levels renumbered: 1 calls only, 2 narrowed (default), 3 BID-derived; the two-word paths deleted | 12 | 25 | 28 | 12 | 17 | 5 | 2 |

The step before old level 4 is old level −1, now level 3.

## Correctness

On darwin/arm64 (Apple Silicon, bootstrap Go 1.26.4) with
`GOEXPERIMENT=decimal`, the default UBD build passes `internal/bid`,
`internal/decimal`, `math/decimal`, `strconv`, `internal/constdecimal`,
`fmt`, `encoding/json` and `encoding/gob` (all `-short`),
`go test cmd/internal/testdir -run Test/decimal`, and
`-run Test/codegen/decimal -all_codegen`, which checks the linux/amd64 and
linux/arm64 code. The operator tests (`TestUBDKernel`, `TestUBDWellFormed`,
`TestOperatorsRawUBD`, `TestOperators`, `TestOperatorZero`) also pass at
levels 1 and 3, and passed at the old levels 1, 2, 3, 9 and −1 before the
renumbering. Each fast path was checked by breaking it on purpose
(its bound, the borrow, the sign of the result, the +0 of a cancellation, the
opposite-sign guard) and seeing these tests fail.

The first UBD commit was also tested on linux/amd64 (the same packages, and
the playground examples printed the same output under both builds); the
later commits have not been run there.

Test changes in `ubd128`: tests that pin XOR-BID memory bits or require
non-canonical BID patterns to survive a round trip now expect UBD /
canonical values, as they already do for DPD on ppc64le. `ubd.go` is
excluded from `internal/bid`'s API-coverage and layout lint tests.

## Not done

- The GDB printer does not know UBD (`TestGdbDecimal` fails).
- `internal/bid/doc.go`'s "Inline fast paths" section still describes the
  BID kernel's contract with the compiler (`addSlow128`, `cCmpFinite128`, …),
  which the UBD build no longer uses.
- `NativeRepresentation` still says `BIDXOR` for decimal128, and
  `Native128frombits` should be removed (Marcelo's API).
