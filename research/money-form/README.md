# A one-word money form for decimal128: prototype study

Raw-word Go prototypes of decimal128 inline fast paths for three layouts,
measured for code per call site and for the share of operations each sends
to the general routine. The study led to `ubdlevel` 10, now level 2 (UBD's add, subtract
and comparison paths narrowed to one-word coefficients), now the default; the
money form itself was not pursued.

## Layouts

- **U:** UBD as the `ubd128` compiler lowered it at `ubdlevel` 9 (old numbering; those paths are now deleted)
  (`ssagen/bid.go`), transcribed to Go source.
- **N:** UBD with the add, subtract and comparison paths narrowed to
  one-word coefficients: one guard, `(xh ^ yh) | xh<<15 == 0`. Same sign
  policy as U (`+` same signs, `-` an effective subtraction). Multiply is
  U's, which already takes coefficients below 2^56 only.
- **C:** the money form. `hi == 0` and `lo = c<<4 | scale`, with c a 60-bit
  two's-complement coefficient and scale = −qExp in [0,15]. Everything else,
  −0 included, is general form: UBD with qExp offset by 8192, so its high
  word is never 0. Signed overflow by the portable xor test (Go source has
  no access to the overflow flag). Multiply takes scales up to 7 each and
  sends a zero product (which may be −0) to the general routine.

## Results (2026-11-21)

Inline code per call site, instructions (bytes), frame excluded, by the
method of `../ubd-size` (`seg.py` here). U comes out 0–5 instructions under
the fork's measured level 9, which validates the transcription.

| | amd64 U | amd64 N | amd64 C | arm64 U | arm64 N | arm64 C |
|---|---|---|---|---|---|---|
| `x + y` | 17 (61) | **11 (35)** | 21 (65) | 11 (44) | **7 (28)** | 14 (56) |
| `x - y` | 36 (139) | 22 (77) | **21 (65)** | 26 (104) | **13 (52)** | 14 (56) |
| `x * y` | **25 (97)** | 25 (97) | 40 (121) | **20 (80)** | 20 (80) | 22 (88) |
| `x == y` | 16 (53) | **11 (34)** | 12 (38) | 11 (44) | **6 (24)** | 8 (32) |
| `x < y` | 20 (65) | 15 (48) | **12 (38)** | 15 (60) | 9 (36) | **8 (32)** |

Share sent to the general routine (`sim/`, 200,000 operations per row):

| workload | op | U | N | C |
|---|---|---|---|---|
| ledger running total, scale 2, 10% refunds | + | 9.9% | 9.9% | 0.0% |
| amount + amount, scale 2, 10% negative | + | 17.9% | 17.9% | 0.0% |
| amount − amount, same | − | 18.1% | 18.1% | 0.0% |
| price × quantity, 1% zero quantities | * | 0.0% | 0.0% | 1.0% |
| amount × tax rate (scale 4), × FX rate (scale 6) | * | 0.0% | 0.0% | 0.0% |
| amount + amount × rate (scale 2 + scale 6) | + | 100% | 100% | 100% |
| compare amounts, < and == | | 14–18% | 14–18% | 0.0% |
| 16–34 digits, one qExp, all positive | + − < | 5.6 / 5.6 / 0.0% | 95% | 98% |
| 16–34 digits, one qExp, 50% negative | + − < | 53 / 53 / 50% | 98% | 98% |
| 16–34 digits | * | 99.2% | 99.2% | 100% |

Conclusions:

- C keeps mixed-sign money operations inline, but its code is not smaller:
  the portable overflow test, the two-word `hi == 0` test and the emulated
  signed 128-bit product cost more than the narrowing saves. N, the same
  narrowing on the UBD layout, is the smallest for add and `==`.
- On money workloads N sends exactly what U sends to the kernel; it gives up
  only coefficients of 2^64 or more (about 20 digits and up).
- Mixed-scale adds go to the general routine in every layout.

## Checks

`sim/` checks every fast-path result against an exact `math/big`
reference (value, exponent, sign including the sign of zero, and the
canonical encoding), on the workloads and on 14.5 million boundary
operations (coefficients at each layout's limits, scales at the money
form's edges, ±0, the specials). 0 errors. Removing C's add overflow test,
its multiply zero test or its scale ≤ 7 test each makes it fail.

## Running

```
G=$GOROOT_UBD/bin/go
for a in amd64 arm64; do
  mkdir -p out/$a
  GOTOOLCHAIN=local GOOS=linux GOARCH=$a $G build -o out/$a/probe ./probe
  $G tool objdump -s '^moneyform/ops\.[UNC]' out/$a/probe > out/$a/objdump.txt
done
python3 seg.py out/amd64/objdump.txt out/arm64/objdump.txt
$G run ./sim
```
