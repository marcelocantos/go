# IEEE 754-2019 behavioral recheck on ubd128

The IEEE 754-2019 audit of 2026-11-22 read the standard against the
proposal and checked the behavior of Marcelo Cantos's XOR-BID toolchain
(5032fd7754): among other probes, a 270,000-case differential of decimal128
and decimal64 against Python's `decimal`. Its findings concern the design
and the API (go-decimal-proposal #38–#47), which have not changed. The
implementation of decimal128 has: the UBD kernel, its tuning, the inline
paths of ubdlevel 2 and 3, and the formatting work. This rechecks the
behavior on ubd128 (37b9ac04c0), through the compiled operators at each
level.

## Method

`gen.py WIDTH N SEED` writes operands and reference results from Python's
`decimal` in the IEEE context of the width: precision 34 or 16, Emax 6144
or 384, clamp (fold-down) on, subnormals, no traps. Operands are exact in
the format and biased toward what the inline paths take: one-word
coefficients at small exponents, pairs at one exponent, cancellations
(x and −x, also at another exponent), members of one cohort, zeros of both
signs, NaN and the infinities, and values at the ends of the exponent
range.

`check.go` (decimal128, or decimal64 with `-tags w64`) parses each case,
applies the compiled operator through a non-inlined function, and compares
coefficient, exponent and sign (strconv's 'b' form), or the bool, int64 or
string:

- `+ − × ÷` and unary `−`; `== != < <= > >=`; built-in `min` and `max`
  (NaN if either is NaN, ties between cohort members and zeros broken by
  totalOrder);
- `Quantize`, and `QuantizeMode` in all six rounding modes;
- `Trunc`, `Floor`, `Ceil`, `Round`, `RoundToEven` (roundToIntegral);
- `FMA`, `Sqrt`, `Remainder` (IEEE remainder), `Mod` (truncated
  remainder);
- `int64(x)` for values in range;
- strconv 'f' at precision −1 against Python's `format(x, "f")`, and a
  round trip of every finite operand through 'g' (value and quantum back)
  and through 'f' at exponents ≤ 0 ('f' writes a positive exponent as
  zeros, 4.35E+3 as "4350", so no notation without an exponent can keep
  it).

Python's `remainder` and `remainder_near` return NaN when the integer
quotient has more digits than the precision (the General Decimal
Arithmetic's "division impossible"); IEEE 754-2019's remainder is exact
for every quotient, and `math/decimal` documents it so. For finite operands
gen.py computes both remainders exactly with integers instead.

## Results

11,000 rounds per width (seeds 7 and 8): 269,678 decimal128 cases and
270,134 decimal64 cases, each run by 12 builds: levels 1, 2 and 3, arm64
natively and amd64 under Rosetta, decimal128 and decimal64.

| build | decimal128 | decimal64 |
|---|---|---|
| arm64, levels 1, 2, 3 | 0 mismatches | 0 mismatches |
| amd64, levels 1, 2, 3 | 0 mismatches | 0 mismatches |

Coverage of what the audit had verified and our code now implements, per
decimal128 run: 2,394 sums and differences with a zero result (52 of them
−0, the clause 6 sign rules); 1,238 `==` between different encodings of
one value; 1,238 `min` and `max` of equal values in different encodings;
2,419 additions at one exponent (level 2's path) and 5,568 at exponents 1
to 14 apart (level 3's). A deliberately wrong expected value is reported.

Not covered here: QuoQuantize, the elementary functions (§9.2; unchanged
kernel code), parsing (unchanged), and the API-level findings, which stand
as filed.

## Running it

    python3 gen.py 128 11000 7 > c128.txt
    python3 gen.py 64 11000 8 > c64.txt
    GOEXPERIMENT=decimal go build -gcflags=all=-d=ubdlevel=3 -o chk128 .
    GOEXPERIMENT=decimal go build -tags w64 -gcflags=all=-d=ubdlevel=3 -o chk64 .
    ./chk128 < c128.txt
    ./chk64 < c64.txt

with the ubd128 toolchain's go; set GOARCH=amd64 for the amd64 build. The
last line is the total; each mismatch is printed (five per operation) with
the result obtained.
