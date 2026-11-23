# ubdlevel 4: the kernel's first paths, promoted inline

Level 4 (`-d=ubdlevel=4`) is level 2's lowering, unchanged, with the
kernel's own first paths promoted inline behind level 2's guards one at a
time. It is built from level 2's measured kernel traffic
(research/ubd-kernel/README.md, "What level 2 sends the kernel") and the
kernel's tuned leaves, not from the XOR-BID-derived level 3, which stays
as a reference until level 4 beats it and is then removed.

Constraints (the user's, 2026-11-23):

- 64-bit coefficients only, for now.
- At most 125 instructions per inline call site on arm64, counted as
  research/ubd-size counts it: the probe function's instructions less its
  frame (stack check, prologue, epilogue, stack-growth stub). amd64's
  count is recorded but not constrained.
- Never slower than level 2 on level 2's own pairs.

## Method

- **Correctness:** `l4diff` (scratch) builds one program at levels 1 and
  4, runs random UBD operand pairs built from their two words (64-bit
  random, small, near powers of ten, zeros, near 2^64; 4% two-word
  coefficients, 0.5% specials; exponents equal, 1–3, up to 20 and up to 30
  apart, and near both ends of the range; 10% x == y), and compares a hash
  of every result. Also the math/decimal and strconv tests built at level
  4, and the codegen tests (level 2).
- **Speed, per operation:** `l4bench` (scratch), the inline operation in a
  loop over 1024 pairs per class, independent operations and running
  totals, built at each level. Built with `-ldflags=-funcalign=4096`: at
  64 the sub-nanosecond loops moved by 35% with where a loop fell against
  a 4 KB page (the level-2 loop, instruction for instruction the same in
  two builds, ran 0.58 or 0.80 ns).
- **Speed, apps:** the seven apps at levels 2, 3 and 4 (`-funcalign=64`),
  interleaved, best and median of 7; outputs compared.
- **Size:** `sitesize.py` (scratch) on a probe built at each level.

## Step 1: add and subtract of one-word coefficients

Behind level 2's guard (one exponent, one sign, x one word), for a pair
of one-word coefficients: the kernel's `ubdAddWordTTE`. x − y is
x + (−y).

- One exponent, opposite signs (x + y) or one sign after the flip
  (x − y): the low words' difference, chosen by a select, or their sum.
- Exponents 1 to 19 apart: a, the operand with the larger exponent, is
  scaled by 10^gap from the table (one 64×64 multiply) and b added or
  subtracted; a difference that borrows out of the high word is b's
  magnitude less a's, sign flipped; an exact zero is +0; the result must
  be below 2^112 (one shift and test; a comparison with MaxCoeff128 built
  its two words as a constant that the compiler hoisted onto level 2's
  fast path, six instructions in every iteration of a running total).
- Everything else, the kernel call.

Level 2's guard is marked likely, as at level 3, so the new code is laid
out of line.

Which operand is a is a branch, with the operands carried through phis.
Tried and dropped:

- Four selects for the swap: single operations 1.90 ns against 2.13, but
  a running total at different exponents 3.38 against 1.60, level 3's
  speed: the selects put the comparison of the high words on the total's
  dependency chain ahead of the multiply.
- The tail written once per order, so that no phi moves are needed: 16
  more instructions on arm64; single operations 6–12% faster, running
  totals 5–23% slower.

Correctness: levels 1 and 4 agree on 10M random pairs (add, subtract and
the six comparisons). math/decimal and strconv pass built at level 4.

Per operation, ns (page-aligned builds, medians of 5):

| | L2 | L3 | L4 |
|---|---|---|---|
| Add, one exponent, one sign (money) | 0.90 | 0.91 | 0.91 |
| Add, 10% negative (cents10neg) | 1.15 | 1.00 | 1.00 |
| Add, amount + rate (gap2) | 2.94 | 1.96 | 2.13 |
| Add, rate + amount (gap2r) | 3.21 | 1.96 | 1.88 |
| Sub, amount − rate (gap2) | 3.35 | 2.81 | 2.42 |
| running total, one exponent (AddChain money) | 0.59 | 0.59 | 0.59 |
| running total, total at −4, amounts at −2 (AddChain gap2r) | 3.18 | 3.19 | 1.61 |
| running balance, crossing zero (SubChain money) | 2.38 | 0.91 | 0.87 |
| running balance, gap2r (SubChain gap2r) | 3.18 | 3.33 | 1.86 |

Size, instructions at the call site (arm64 / amd64): add 8 / 12 at
level 2, 80 / 110 at level 3, 86 / 111 at level 4; subtract 14 / 25,
88 / 126, 80 / 114.

Apps, best of 7 (medians agree within 1–3 points):

| app | L2 → L4 | L3 → L4 |
|---|---|---|
| tax ca | −4.3% | +5.1% |
| risk | −3.3% | −0.4% |
| tax nc | −3.3% | +1.1% |
| telco | −1.5% | +0.4% |
| tax fed | −1.2% | +4.3% |
| banking | −0.9% | +1.0% |
| euro | +0.3% | +1.0% |

Level 3 is still ahead on the tax apps: it also inlines comparisons at
different exponents and integral rounding, which level 4 has not reached.

`cmd/compile/internal/ssagen`'s TestIntrinsics fails at this branch's
base as well (it does not list the `Quantize[shape]` intrinsics); left
alone.

## Step 2: multiply and divide of one-word coefficients

**Multiply.** Behind level 2's guard (both factors below 2^56, exponent
sum in range): the kernel's `ubdMulShort128` for one-word factors, with
the product tested instead of the factors: below 2^112, exact at the sum
of the exponents. It takes the factors of 17 to 19 digits whose product
still fits. Level 2's counts show none in the apps (their kernel
multiplies are two-word), so this is for code that has them: 1.50 ns
against 2.67 at levels 2 and 3. Site: 33 instructions on arm64 (level
2: 20), 52 on amd64.

**Divide.** Level 2 has no inline divide. Counted at level 2 (scratch
counting toolchain), divides of two one-word coefficients:

| app | a % b == 0 | a == 0 | b a power of ten | inexact |
|---|---|---|---|---|
| banking | 1348 | 98 | 496 | 870 |
| euro | 162 | 0 | 8 | 228 |
| risk | 6 | 28 | 10 | 2579 |
| tax fed, nc | 0–3 | 0 | 0 | 3–6 |

Promoted: the kernel's exact quotient where b divides a: one `UDIV`,
the remainder zero, the exponent difference in range; the quotient is
exact at the preferred exponent (the kernel's power-of-ten and exact
paths give the same for such a pair). A zero dividend is the same path.
The inexact divide stays in the kernel: its long division (up to four
dependent divides) would not fit the site limit and would save only the
call.

| ns | L2 | L3 | L4 |
|---|---|---|---|
| amount / a count that divides it (Div exact) | 4.34 | 4.33 | 1.33 |
| amount / 3 (Div inexact) | 10.65 | 10.64 | 11.49 |

The inexact divide pays the `UDIV` before the call: +0.84 ns, 8%. Site:
21 instructions on arm64, 34 on amd64.

Levels 1 and 4 agree on 5M random multiply/divide pairs, one in six with
x's coefficient a multiple of y's, and on 3M pairs of all five
operations. Apps, against step 1 (best of 7): banking −0.6%, risk +1.1%,
tax nc +1.7%, the rest within ±0.6%; medians −1.7% to +0.8%. Divide is
a small share of every app.

**For the user:** the exact divide gains banking and euro little and
costs risk's inexact divides about 1%. Kept for now; it can be dropped
without touching the rest.

## Step 3: comparisons of one-word coefficients

Behind level 2's compare guard (one exponent, one sign, one word): the
kernel's `ubdCmpWord` for a pair of one-word coefficients at most 19
places apart (wider gaps, none in the apps' counts, take the call).

First written as the kernel's code, branch-free to a three-way result
and then tested: `<` 3.35 ns against level 3's 2.58, a long serial
chain. Rewritten for the one predicate the site asks: a (the larger
exponent) times 10^gap is compared with b once each way, P > b and
P < b, one borrow chain each; one select picks which is x < y, from
whether x is a and the sign; signs that differ decide alone unless both
values are zero; `==` is one sign and P == b, or both zero.
Branch-free past the guards, since the outcome is the data's.

| ns | L2 | L3 | L4 |
|---|---|---|---|
| `<`, one exponent, one sign (money) | 1.04 | 1.03 | 1.03 |
| `<`, 10% negative (cents10neg) | 1.38 | 1.11 | 1.15 |
| `<`, exponents 2 apart (gap2) | 4.85 | 2.58 | 2.60 |
| `<`, exponents −1..−6 (mixexp) | 6.57 | 2.43 | 2.60 |
| `==`, gap2 | 5.04 | 2.25 | 1.89 |
| running maximum, gap2neg | 1.25 | 1.03 | 1.10 |

Site, arm64: `<` 56 instructions (level 2: 10, level 3: 77), `==` 38
(7, 64), `<=` 60 (10, 78).

Levels 1 and 4 agree on 10M random pairs, one in eight a member of x's
cohort or one unit from it, all six comparisons.

Apps, best of 7: against step 2, tax fed −3.0%, risk −0.9%, tax ca
−0.9%, the rest within ±0.6% (medians −4.3% to +2.0%); against level 3,
−2.1% (tax nc) to +2.9% (tax ca), medians −0.5% to +1.9%: even.

## Step 4: integral rounding of one-word coefficients

Level 4 runs integral rounding through the inline word paths level 2
gives quantize (which are level 3's): x with qExp ≥ 0 is x, a one-word x
with a fraction of 1 to 19 digits is one division by the power of ten,
rounded in the mode; the rest call the kernel. The kernel's word path
does the same (`ubdQuantizeWord` against 1E0). Level 2 had only the
qExp ≥ 0 exit.

| ns | L2 | L3 | L4 |
|---|---|---|---|
| RoundToEven, money | 3.21 | 1.40 | 1.40 |
| Trunc, money | 3.20 | 1.16 | 1.15 |

Site, arm64: `Round` 22, `Trunc` 15, `RoundToEven` 24 (level 2: 6–7).
Levels 1 and 4 agree on 3M random values rounded 13 ways (five integral
roundings, quantize in eight forms) and on the 43 edge values.

Apps, best of 7, against step 3: −1.0% (euro) to +1.4% (tax ca);
against level 3: −0.9% to +2.1%, medians −0.9% to +1.4%.
