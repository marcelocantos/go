# UBD decimal128: inline code size vs XOR-BID

Experiment on Marcelo Cantos's Go decimal fork
([marcelocantos/go](https://github.com/marcelocantos/go), branch `decimal`),
pinned at `5032fd77`, the commit his proposal repository builds
([go-decimal-proposal](https://github.com/marcelocantos/go-decimal-proposal),
`toolchain.lock`).

- **Baseline (XOR-BID):** `5032fd77` unchanged. decimal128 is stored as its
  IEEE BID encoding with the high word XORed with `0x3040000000000000` (the
  encoding of +0E+0), so that all bits clear is the zero value.
- **UBD (branch `ubd128`):** one commit on `5032fd77`. decimal128 is stored
  in the UBD layout
  ([whitepaper](https://migueldecimal128.github.io/decimal128-www/whitepapers/ubd.html)):
  qExp in bits 127..114 (two's complement, unbiased), sign in bit 113,
  binary coefficient in bits 112..0. All bits clear is +0E+0 with no mask.
  decimal64 stays XOR-BID in both builds.

The goal is generated code size, not speed.

## What changed in `ubd128`

- `internal/bid/ubd.go`: UBD constants, `UBDFromBID128` / `BIDFromUBD128`,
  and kernel entry points (`ubdAddSlow128`, `ubdMulSlow128`, ...) that
  convert to BID, call the unchanged plain-BID kernel, and convert back.
- `internal/bid/native.go`: decimal128's native form is UBD; its zero mask is 0.
- `cmd/compile/internal/ssagen/bid.go`, `bid_compare.go`, `bid_round.go`:
  the decimal128 inline lowerings (add/sub, mul, compare, compare with zero,
  Quantize/Round, kernel calls) use UBD bit positions, with the same
  structure as the baseline. Checks UBD makes redundant are dropped: the
  second-form/special guard (a 64-bit coefficient rules out specials, whose
  coefficients begin 0x1F), the zero-value XOR around kernel calls, and the
  exponent bias (the exponent is one signed shift).
- `cmd/compile/internal/ssacompile/softdecimal.go`: negation flips bit 49.
- `cmd/compile/internal/ir/decimal.go`: constants are encoded as UBD.
- `internal/decimal/arithmetic.go`: `IsNaN`, `IsInf`, `Signbit`, `Abs`,
  `Copysign` read UBD's decimal128 word.

The kernel (`internal/bid`) is untouched and still works on plain BID.

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
```

where each GOROOT is a toolchain built with `src/make.bash` (bootstrap: Go
1.26 or later) from `5032fd77` and from `ubd128` respectively. It builds the
probes for amd64 and arm64 with `GOEXPERIMENT=decimal` and prints the tables.

## Results (2026-11-20)

### Inline code per call site (XOR-BID → UBD, decimal128)

| Op | amd64 instr | amd64 bytes | arm64 instr | arm64 bytes |
|---|---|---|---|---|
| `x + y` | 169 → 134 (−21%) | 714 → 491 (−31%) | 119 → 104 (−13%) | 476 → 416 (−13%) |
| `x - y` | 179 → 136 (−24%) | 773 → 507 (−34%) | 126 → 106 (−16%) | 504 → 424 (−16%) |
| `x * y` | 84 → 57 (−32%) | 369 → 211 (−43%) | 65 → 50 (−23%) | 260 → 200 (−23%) |
| `x / y` (call only) | 17 → 12 (−29%) | 64 → 35 (−45%) | 15 → 13 (−13%) | 60 → 52 (−13%) |
| `x == y` | 169 → 108 (−36%) | 708 → 388 (−45%) | 131 → 95 (−27%) | 524 → 380 (−27%) |
| `x < y` | 201 → 124 (−38%) | 795 → 435 (−45%) | 152 → 104 (−32%) | 608 → 416 (−32%) |
| `x == 0` | 30 → 22 (−27%) | 135 → 84 (−38%) | 31 → 23 (−26%) | 124 → 92 (−26%) |
| `x > 0` | 41 → 31 (−24%) | 186 → 116 (−38%) | 40 → 31 (−22%) | 160 → 124 (−22%) |
| `Quantize` | 119 → 97 (−18%) | 491 → 341 (−31%) | 81 → 78 (−4%) | 324 → 312 (−4%) |
| `Round` | 58 → 43 (−26%) | 248 → 151 (−39%) | 42 → 32 (−24%) | 168 → 128 (−24%) |

Per extra `+` in a longer expression (`scale/`): amd64 162 → 127 instructions
(695 → 497 bytes), arm64 109 → 87 instructions (437 → 348 bytes).

Notes:

- `x == y`, `x < y`: part of the saving comes from moving infinity/NaN
  comparisons out of line to a kernel call. BID handles them inline; UBD's
  64-bit-coefficient check already rejects them. About 38–50 instructions
  (160–190 bytes) on amd64.
- Outside this table: the UBD build adds the conversion entry points to the
  kernel once per program (0.7–2.0 KB on amd64, by which operations a
  program uses), and its slow path converts UBD↔BID around each kernel call
  where the baseline XORs. A UBD-native kernel would remove both.

## Correctness

On linux/amd64 with `GOEXPERIMENT=decimal`, the UBD build passes:
`go test cmd/internal/testdir -run Test/decimal`; `internal/bid` (including
`TestOperators`, compiled operators vs the kernel, 10^5 cases each);
`internal/decimal`, `math/decimal`, `internal/constdecimal`, `strconv`,
`fmt`, `encoding/json`, `encoding/gob`. The playground examples print the same
output under both builds.

Test changes in `ubd128`: tests that pin XOR-BID memory bits or require
non-canonical BID patterns to survive a round trip now expect UBD /
canonical values, as they already do for DPD on ppc64le. `ubd.go` is
excluded from `internal/bid`'s API-coverage and layout lint tests.

On darwin/arm64 (Apple Silicon, bootstrap Go 1.26.4), the UBD build passes
`internal/bid` (including `TestOperators`), `internal/decimal`,
`math/decimal`, `strconv` (all `-short`) and
`go test cmd/internal/testdir -run Test/decimal`. The measurement run there
(`GOOS=linux ./measure.sh ...`) reproduces the tables above exactly.

Not done: the GDB printer does not know UBD (`TestGdbDecimal` fails).
