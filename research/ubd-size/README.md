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

- **`x + y`:** decimal128-go's first-level path (`d128_add_rnd_ctxnull`):
  equal qExp and sign, and the high 49 coefficient bits of the operands
  summing below 2^48. y's coefficient is added onto x's whole stored value
  with one add-with-carry pair, so x's qExp and sign ride along. Every other
  pair calls `ubdAddTTE128`.
- **`x - y`:** decimal128-go's second-level path (the first test of
  `d128_add_slow_rnd_ctxnull`), for operands of one sign and one qExp, which
  are an effective subtraction: bit 48 clear in both high words (so both
  coefficients are below 2^112, neither is special, and the difference is
  exact), the magnitudes subtracted, a negative difference negated with y's
  sign, an exact zero +0. Every other pair calls `ubdSubTTE128`.
- **`x * y`:** decimal128-go's first-level multiply path, narrowed: both
  coefficients below 2^56 (the high words' coefficient bits clear and the low
  words below 2^56, one OR'd test) and the qExp sum in range (one unsigned
  compare). The product is then below 2^112 and needs no check after the
  64×64 multiply; the header is the sign XOR and the qExp sum. Every other
  pair calls `ubdMulTTE128`. Taking full 64-bit coefficients would cost a
  test of the product's high word after the multiply: 2 more instructions on
  amd64 (30), 1 on arm64 (21).
- **Everything else** (`/`, comparisons, comparisons with zero,
  `decimal.Quantize`, `decimal.Round` and the other integral roundings) is
  the kernel call alone.

`-d=ubdlevel=N` selects how much is inlined, so that each fast path can be
measured against the call alone:

| Level | Inline fast paths |
|---|---|
| 1 | none: every operation is its kernel call |
| 2 | first-level add path, at `+` and `-` |
| 3 | first- and second-level add paths, at both `+` and `-` |
| 4 | first level at `+`, second level at `-` |
| **5** (default, `0`) | level 4, and the first-level multiply path at `*` |
| −1 | the inline paths derived from the BID lowering (see History) |

From level 4, `5.00 + 3.00` and `5.00 - 3.00` are inline; `5.00 + -3.00` and
`5.00 - -3.00`, operands of opposite signs, call the kernel.

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

- Each decimal128 is passed as `UBD128{Lo, Hi}`, **low word first**: the
  order in which it is laid out and passed in registers, so a call site moves
  nothing into place and the result arrives where it is returned.
- The operators round ties to even and call the TTE entries, so a call site
  sets no mode register. Quantize and integral rounding call TTE when the mode
  is the constant ToNearestEven (`decimal.Quantize`, `decimal.RoundToEven`)
  and Rnd otherwise. The Rnd entries of add, subtract, multiply and divide have
  no caller yet.
- The result of each entry is the exported BID function's on the operands'
  BID encodings, converted to UBD, and it is canonical UBD.
- Today each entry converts its operands to BID, calls the unchanged BID
  kernel and converts back. A kernel that works on UBD itself replaces those
  bodies and nothing else.

### Well-formed UBD

UBD's 14-bit qExp field holds −8192..8191, of which decimal128 uses
−6176..6111 and the special tag 7168..8191. The 3072 values between have no
meaning, nor has the special tag without the 0x1F coefficient prefix. (BID
has no such gap: its exponent range fills its three non-`11` prefixes
exactly.) UBD is internal and never read from an external source, so a
malformed pattern can only come from a bug or from unsafe code:

- `UBDWellFormed128` states the rule; the contract covers well-formed patterns
  only, and the inline paths do not check for malformed ones.
- `math/decimal.Native128frombits`, the one way to write arbitrary words into
  a decimal128 without unsafe, stores a malformed pattern as a positive quiet
  NaN. Its comment says it should be removed.
- The UBD tests fail at once on a malformed operand or result.

A finite coefficient above 10^34 − 1 is well-formed (it reads as zero, as
BID's non-canonical coefficients do).

## What changed in `ubd128`

- `internal/bid/ubd.go`: UBD constants, `UBDFromBID128` / `BIDFromUBD128`,
  `UBDWellFormed128`, the kernel contract and its entries.
- `internal/bid/native.go`: decimal128's native form is UBD; its zero mask
  is 0.
- `cmd/compile/internal/ssagen/bid.go`, `dpd.go`, `bid_round.go`,
  `intrinsics.go`: the UBD lowering (`ubdLevel`, `bidCallOnly`, `ubdAddFast`)
  and the kernel calls; `math/decimal.Quantize[shape]` is recognized with a
  constant mode, as `Round` and its relatives already were. The BID-derived
  inline paths in `bid.go`, `bid_compare.go` and `bid_round.go` use UBD bit
  positions and remain for level −1.
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
```

where each GOROOT is a toolchain built with `src/make.bash` (bootstrap: Go
1.26 or later) from `5032fd77` and from `ubd128` respectively. It builds the
probes for amd64 and arm64 with `GOEXPERIMENT=decimal` and prints the tables;
`UBD_GCFLAGS` is passed to the UBD builds. The tables below were measured on
darwin/arm64 with `GOOS=linux`.

## Results (2026-11-20)

### Inline code per call site, default (level 5)

| Op | amd64 instr | amd64 bytes | arm64 instr | arm64 bytes |
|---|---|---|---|---|
| `x + y` | 169 → 17 (−90%) | 714 → 60 (−92%) | 119 → 12 (−90%) | 476 → 48 (−90%) |
| `x - y` | 179 → 41 (−77%) | 773 → 150 (−81%) | 126 → 26 (−79%) | 504 → 104 (−79%) |
| `x * y` | 84 → 28 (−67%) | 369 → 103 (−72%) | 65 → 20 (−69%) | 260 → 80 (−69%) |
| `x / y` | 17 → 2 (−88%) | 64 → 6 (−91%) | 15 → 3 (−80%) | 60 → 12 (−80%) |
| `x == y` | 169 → 5 (−97%) | 708 → 14 (−98%) | 131 → 6 (−95%) | 524 → 24 (−95%) |
| `x < y` | 201 → 5 (−98%) | 795 → 14 (−98%) | 152 → 4 (−97%) | 608 → 16 (−97%) |
| `x == 0` | 30 → 7 (−77%) | 135 → 18 (−87%) | 31 → 8 (−74%) | 124 → 32 (−74%) |
| `x > 0` | 41 → 9 (−78%) | 186 → 24 (−87%) | 40 → 8 (−80%) | 160 → 32 (−80%) |
| `Quantize` | 119 → 2 (−98%) | 491 → 6 (−99%) | 81 → 3 (−96%) | 324 → 12 (−96%) |
| `Round` | 58 → 3 (−95%) | 248 → 11 (−96%) | 42 → 4 (−90%) | 168 → 16 (−90%) |

XOR-BID → UBD. Per extra `+` in a longer expression (`scale/`): amd64 162 →
23 instructions (695 → 96 bytes), arm64 109 → 17 (437 → 68).

A call site of `x / y` on amd64 is the 5-byte `CALL ubdDivTTE128` and a
1-byte `NOP`; the operands are already in the registers the entry takes.
`x == y` is the call and the test of its result.

### `x + y` and `x - y` by level

| Level | `+` amd64 | `-` amd64 | `+` arm64 | `-` arm64 | per extra `+`, amd64 |
|---|---|---|---|---|---|
| 1: call alone | 2 (6 B) | 2 (6 B) | 3 | 3 | 5 (29 B) |
| 2: first level | 17 (60 B) | 21 (77 B) | 12 | 14 | 23 (96 B) |
| 3: both levels | 52 (190 B) | 55 (205 B) | 33 | 35 | 57 (227 B) |
| **4, 5 (default)** | **17 (60 B)** | **41 (150 B)** | **12** | **26** | **23 (96 B)** |
| −1: BID-derived | 101 (383 B) | 102 (388 B) | 74 | 76 | 111 (446 B) |

The first-level path on amd64:

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

- **Frames.** In a function that makes no other call, such as the probe's
  `IsZero`, the kernel call also forces a stack frame (about 12 instructions
  on amd64) that inline code did not need. The tables exclude it; in a
  function that already calls something it costs nothing more.
- **Kernel entries.** The UBD entries and the two conversions are about
  2.1 KB on amd64 (2.0 KB on arm64) in a program that uses every operation,
  once per program. Each TTE entry (70 bytes on amd64) calls its Rnd entry.
- **Speed.** Not measured. Every operation without a fast path now pays for a
  call, and until the kernel works on UBD itself each call also converts
  UBD ↔ BID.

### History

The UBD build reached the default in steps, each measured with this harness
(amd64 instructions per call site):

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
| Multiply narrowed to 2^56 (`b5c2edb4`); level 5 default | 17 | 41 | 28 | 5 | 5 | 7 | 2 |

The step before level 4 is level −1.

## Correctness

On darwin/arm64 (Apple Silicon, bootstrap Go 1.26.4) with
`GOEXPERIMENT=decimal`, the default UBD build passes `internal/bid`,
`internal/decimal`, `math/decimal`, `strconv`, `internal/constdecimal`,
`fmt`, `encoding/json` and `encoding/gob` (all `-short`),
`go test cmd/internal/testdir -run Test/decimal`, and
`-run Test/codegen/decimal -all_codegen`, which checks the linux/amd64 and
linux/arm64 code. The operator tests (`TestUBDKernel`, `TestUBDWellFormed`,
`TestOperatorsRawUBD`, `TestOperators`, `TestOperatorZero`) also pass at
levels 1, 2, 3 and −1. Each fast path was checked by breaking it on purpose
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

- **A kernel that works on UBD** (replacing the bodies of the contract's
  entries, which now convert to and from BID).
- **Benchmarks** of the default against level −1 and the baseline.
- The GDB printer does not know UBD (`TestGdbDecimal` fails).
- `internal/bid/doc.go`'s "Inline fast paths" section still describes the
  BID kernel's contract with the compiler (`addSlow128`, `cCmpFinite128`, …),
  which the UBD build no longer uses.
- `NativeRepresentation` still says `BIDXOR` for decimal128, and
  `Native128frombits` should be removed (Marcelo's API).
