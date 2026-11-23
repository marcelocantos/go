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
