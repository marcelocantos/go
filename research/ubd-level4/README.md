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
- (2026-11-23, at the user's request) The swap rearranged so that one
  order needs no moves: the gap path's refusals (a gap past 19, a
  result of 2^112 or more) call the add entry with a and b as the path
  holds them (two finite words, whose exact sum does not depend on their
  order; for x − y, y's sign already flipped, as `ubdSubTTE128` does), so
  x and y are not live past the swap; the moves went from 4 + 8 to 0 + 6,
  and the swap branch's prediction picks which order has none. The add
  site fell from 86 to 80 instructions (level 3's size), and single
  operations gained 2–10% (amount − rate 2.43 → 2.20 ns). But running
  totals lost 8–23% (rate-and-amount total 1.60 → 1.86 ns, a balance
  crossing zero 0.87 → 1.07): with the second call's result registers,
  the allocator moved the loop's total to another register, and the
  same-exponent path, which the change does not touch, now ends in a move
  and a jump to a join block where it ended in a jump back to the loop.
  Running totals are what the apps do, so the tail stays as it was.

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

**Dropped (the user's decision, 2026-11-23):** the exact divide gained
banking and euro little and cost every inexact divide its `UDIV` (risk
about 1%). Level 4's divide is the kernel call again, as at level 2.

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

### Fix: comparisons crashed the compiler on amd64

Step 3's comparison selected between bools with CondSelect, which arm64
lowers and amd64 does not ("not lowered: CondSelect BOOL"); the
differential test had run on arm64 only. The orders are now kept as 0 or
1 in a word (the borrows already are) and chosen by CondSelect on words,
with one bool formed at the end. That is also faster than the arm64
version it replaces: `<` at exponents 2 apart 2.54 ns (was 2.60; level 3
2.58), and 54 instructions on arm64 (was 56). The differential tests now
run for amd64 too (under Rosetta): levels 1 and 4 agree on 3M pairs of
all five operations and on the rounding test.

## Call-site instructions, arm64 (2026-11-23, after step 4)

| op | L2 | L3 | L4 |
|---|---|---|---|
| `x + y` | 8 | 80 | 86 |
| `x - y` | 14 | 88 | 80 |
| `x * y` | 20 | 20 | 33 |
| `x / y` | 3 | 3 | 3 (21 with the exact divide, dropped) |
| `x == y` | 7 | 64 | 38 |
| `x != y` | 8 | 67 | 39 |
| `x < y` | 10 | 77 | 54 |
| `x <= y` | 10 | 78 | 57 |
| `x == 0` | 4 | 4 | 4 |
| `x > 0` | 10 | 10 | 10 |
| `Quantize(x, y)` | 30 | 44 | 30 |
| `Quantize(x, 0.01)` | 29 | 42 | 29 |
| `QuantizeMode(x, y, ToZero)` | 22 | 36 | 22 |
| `QuantizeMode`, variable mode | 55 | 68 | 55 |
| `Round` | 7 | 22 | 22 |
| `Trunc` | 7 | 15 | 15 |
| `RoundToEven` | 6 | 24 | 24 |
| `int64(x)` | 12 | 12 | 12 |
| `uint64(x)` | 4 | 4 | 4 |

## Which cases level 3 and level 4 handle (2026-11-23, after step 4)

Counted as in research/ubd-kernel ("What level 2 sends the kernel"): the
scratch counting toolchain, moved to this branch, points the compiler's
kernel symbols at counting wrappers, which level 3's fallbacks reach
too. Each app run once at levels 2, 3 and 4. Kernel calls (quantize mode
counts and the divide sub-classes left out):

| app | L2 | L3 | L4 | where L3 and L4 differ |
|---|---|---|---|---|
| banking | 32,790 | 17,254 | 17,254 | nowhere |
| risk | 13,653 | 5,603 | 5,672 | 69 quantizes of a word to a finer exponent (level 3's scale-up path, which level 4 leaves out); 40 two-word `==` reach `ubdCompare128` at level 3 and `ubdEqual128` at level 4 |
| euro | 2,420 | 1,200 | 1,200 | nowhere |
| tax fed | 347 | 14 | 14 | nowhere |
| tax ca | 62 | 0 | 0 | nowhere |
| tax nc | 63 | 6 | 6 | nowhere |
| telco | 6 | 0 | 0 | nowhere |

What both still send the kernel:

| app | calls |
|---|---|
| banking | two-word add 1,768, sub 2,380, mul 1,980, div 4,156, integral rounding 4,040; word divides 2,812; max 118 |
| risk | word divides 2,623; two-word quantize 2,380, add 80, sub 200, mul 240, `==` 40, integral rounding 40 |
| euro | two-word sub 206, div 196, integral rounding 400; word divides 398 |
| tax | 6–14 a run: word divides, min |

So the two levels inline the same cases in these apps. They differ in
how fast and how large the inline paths are: level 4 is faster on
running totals at different exponents (1.61 against 3.19 ns) and on
`==` (1.88 against 2.25), even elsewhere, and smaller at most sites. The
apps cannot show that: most of their operations take level 2's paths,
the same code at both levels; the inline operations are about a tenth
of a profile and formatting, parsing and allocation 40–60%; and no
kernel function appears in the level-4 profiles. Their remaining kernel
traffic is two-word coefficients (outside the 64-bit constraint) and
divides (an inexact divide's long division does not fit the site
limit).

## What level 4 inlines beyond level 2 (after step 4, exact divide dropped)

All for one-word coefficients (at most 19 digits). Each case added is the
kernel's own first path for the operation. Instructions at the call site
on arm64, from the probe (function less frame, nothing else live):

| operation | inline at level 2 | added at level 4 | still the kernel call at level 4 | arm64 instructions L2 → L4 |
|---|---|---|---|---|
| `x + y` | one exponent, one sign | one exponent, opposite signs; exponents 1–19 apart, any signs | exponents more than 19 apart; a result of 2^112 or more; two-word coefficients; NaN, infinity | 8 → 86 |
| `x - y` | one exponent, one sign | one exponent, opposite signs; exponents 1–19 apart, any signs | as for `x + y` | 14 → 80 |
| `x * y` | both factors below 2^56, exponent sum in range | factors of 17–19 digits whose product is below 2^112 | products of 2^112 or more; two-word factors; exponent sum out of range | 20 → 33 |
| `==` / `!=` | one exponent, one sign; a constant zero | one exponent, opposite signs; exponents 1–19 apart, any signs (equal values at different exponents, zeros of either sign) | exponents more than 19 apart; two-word coefficients; NaN | 7 → 38 / 8 → 39 |
| `<` / `<=` (`>` / `>=` alike) | as for `==` | as for `==` | as for `==` | 10 → 54 / 10 → 57 |
| `Round` / `Trunc` / `RoundToEven` (`Floor`, `Ceil` alike) | no fraction (exponent ≥ 0) only | a fraction of 1–19 digits: one division, rounded in the mode | two-word coefficients; NaN, infinity | 7 → 22 / 7 → 15 / 6 → 24 |

The same at both levels: quantize (level 2 already has the same-exponent
and coarser-exponent paths; a finer exponent calls the kernel), `x / y`
(the kernel call), `int64(x)`, `uint64(x)`, `min`, `max`, and the
comparisons with a constant zero (`x == 0` 4, `x > 0` 10). `>`, `>=`,
`Floor` and `Ceil` were not probed separately; they are the same code
with the operands or the mode changed.

## Step 5: quantize to a finer exponent

Level 4 keeps level 3's scale-up path for quantize, which level 2 leaves
to the kernel: a one-word x up to 19 places coarser than y is multiplied
by 10^k, exact while the product is a word (5 quantized to cents is
5.00). The kernel's `ubdQuantizeWord` does the same. It was the one case
level 3 inlines that level 4 did not (69 calls a run in risk; too few to
time in the apps). Site, arm64: `Quantize` 30 → 44, `Quantize(x, 0.01)`
29 → 42, `QuantizeMode(ToZero)` 22 → 36, variable mode 55 → 68: level
3's sizes. Levels 1 and 4 agree on 3M random quantizes (exponents from
23 places finer to 42 coarser) in 13 roundings, on arm64 and amd64, and
on the edge values.

## Step 6: min and max

The language's `min` and `max` on decimal128 are the kernel call at
levels 2 and 3 (`ubdMin128`, `ubdMax128`). At level 4:

- One exponent, one sign, one-word coefficients (level 2's comparison
  guard): the low words, XORed with the sign mask, order the values; one
  select picks the operand. Equal low words are one pattern: x.
- Otherwise, one-word coefficients at most 19 places apart: the
  comparison's word order (`ubdWordOrder`, factored out of step 3's
  `ubdCmpWord4`), x < y and y < x, and two selects for the operand.
- Equal values that are not one pattern (−0 and +0, members of one
  cohort), and every other pair: the kernel call, whose totalOrder
  decides them.

| ns | L3 | L4 |
|---|---|---|
| `min`, amounts at one exponent (money) | 5.01 | 1.14 |
| `min`, 10% negative (cents10neg) | 5.01 | 1.36 |
| `min`, exponents 2 apart (gap2) | 5.01 | 2.91 |
| running `max`, money | 8.76 | 1.10 |
| running `max`, gap2neg | 8.57 | 1.42 |
| running `max`, exponents −1..−6 | 9.52 | 4.84 |

Site, arm64: 66 instructions (levels 2 and 3: 3). Banking calls `max`
118 times a run and tax `min` a few times: too few to time in the apps.
Levels 1 and 4 agree on 10M random pairs (arm64) and 3M (amd64), cohort
members and zeros of either sign among them; the comparisons, which now
share `ubdWordOrder`, are unchanged in size (`<` 54, `==` 38) and speed.
