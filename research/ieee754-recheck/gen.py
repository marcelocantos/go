#!/usr/bin/env python3
# gen.py WIDTH N SEED > cases.txt
#
# Operands and reference results for check.go, from Python's decimal module
# in the IEEE 754-2019 decimal128 (WIDTH 128) or decimal64 (WIDTH 64)
# context: precision 34 or 16, Emax 6144 or 384, clamp (fold-down) on,
# subnormals, no traps. Each line is
#
#   op <TAB> a <TAB> b <TAB> c <TAB> mode <TAB> expected
#
# with unused fields "-". Results are written as strconv's 'b' format gives
# them ("-150e-2", "0e+0", "NaN", "+Inf"), comparisons as true/false, int64
# conversions as integers and the 'f' format as the string itself.
import decimal as D
import random
import sys

W, N, SEED = int(sys.argv[1]), int(sys.argv[2]), int(sys.argv[3])
PREC, EMAX = (34, 6144) if W == 128 else (16, 384)
EMIN = 1 - EMAX
ETINY = EMIN - PREC + 1
EMAXQ = EMAX - PREC + 1  # the largest exponent of an unclamped encoding
CTX = D.Context(prec=PREC, Emax=EMAX, Emin=EMIN, rounding=D.ROUND_HALF_EVEN, clamp=1, traps=[])
D.setcontext(CTX)
r = random.Random(SEED)

MODES = {  # math/decimal RoundingMode value -> Python rounding
    0: D.ROUND_HALF_EVEN, 1: D.ROUND_HALF_UP, 2: D.ROUND_DOWN,
    3: D.ROUND_UP, 4: D.ROUND_FLOOR, 5: D.ROUND_CEILING,
}


def mk(neg, c, e):
    return D.Decimal((1 if neg else 0, tuple(int(ch) for ch in str(c)), e))


def coeff(maxdig):
    n = r.randint(1, maxdig)
    return r.randrange(10 ** (n - 1), 10 ** n) if r.random() < 0.9 else r.choice([10 ** (n - 1), 10 ** n - 1])


def operand():
    """A value the format holds exactly, biased toward what the inline
    paths take: one-word coefficients at small exponents."""
    k = r.random()
    neg = r.random() < 0.3
    if k < 0.03:
        return r.choice([D.Decimal("NaN"), D.Decimal("Infinity"), D.Decimal("-Infinity")])
    if k < 0.10:
        return mk(neg, 0, r.randint(-12, 4) if r.random() < 0.8 else r.randint(ETINY, EMAXQ))
    if k < 0.55:  # money-like: one word, exponent -8..2
        return mk(neg, coeff(min(PREC, 12)), r.randint(-8, 2))
    if k < 0.80:  # one-word coefficient of any length, exponent -25..10
        return mk(neg, coeff(min(PREC, 19)), r.randint(-25, 10))
    if k < 0.95:  # any length
        return mk(neg, coeff(PREC), r.randint(-40, 20))
    # extremes: overflow, underflow and subnormal results
    c = coeff(PREC)
    nd = len(str(c))
    e = r.choice([r.randint(ETINY, ETINY + 40), r.randint(EMAXQ - 40, EMAXQ)])
    if e + nd - 1 > EMAX:
        e = EMAX - nd + 1
    return mk(neg, c, e)


def pair():
    a = operand()
    k = r.random()
    if k < 0.15 and a.is_finite():  # the same exponent, as level 2's paths take
        b = operand()
        if b.is_finite():
            t = b.as_tuple()
            b = D.Decimal((t.sign, t.digits, a.as_tuple().exponent))
        return a, b
    if k < 0.25 and a.is_finite():  # cancellation: x and -x, perhaps at another exponent
        b = a.copy_negate()
        if r.random() < 0.5:
            b = b.quantize(D.Decimal((0, (1,), a.as_tuple().exponent - r.randint(0, 5))))
        return a, b
    if k < 0.35 and a.is_finite():  # equal values at other quanta (cohort members)
        e = a.as_tuple().exponent - r.randint(1, 4)
        b = a.quantize(D.Decimal((0, (1,), e)))
        if not b.is_nan():
            return (a, b) if r.random() < 0.5 else (b, a)
    return a, operand()


def bfmt(x):
    if x.is_nan():
        return "NaN"
    if x.is_infinite():
        return "-Inf" if x.is_signed() else "+Inf"
    t = x.as_tuple()
    digits = "".join(map(str, t.digits)).lstrip("0") or "0"
    return ("-" if t.sign else "") + digits + "e" + ("+" if t.exponent >= 0 else "") + str(t.exponent)


def ffmt(x):
    if x.is_nan():
        return "NaN"
    if x.is_infinite():
        return "-Inf" if x.is_signed() else "+Inf"
    return format(x, "f")


def isnan(*xs):
    return any(x.is_nan() for x in xs)


def cmp(op, a, b):
    if isnan(a, b):
        return op == "ne"
    return {"eq": a == b, "ne": a != b, "lt": a < b, "le": a <= b, "gt": a > b, "ge": a >= b}[op]


def minmax(op, a, b):
    # Go's built-in min and max on decimals: NaN if either is NaN; ties
    # between members of one cohort (and +0, -0) broken by totalOrder.
    if isnan(a, b):
        return D.Decimal("NaN")
    if a != b:
        return (min if op == "min" else max)(a, b)
    lo, hi = (a, b) if a.compare_total(b) <= 0 else (b, a)
    return lo if op == "min" else hi


def exact_rem(a, b, near):
    """IEEE 754 remainder (near) or the truncated remainder (math/decimal's
    Mod), exact for every quotient. Python's decimal returns NaN when the
    integer quotient has more digits than the precision (the General Decimal
    Arithmetic's "division impossible"); IEEE 754-2019 has no such limit, so
    finite operands are computed here with integers. The result's exponent is
    the smaller of the operands', and a zero result has x's sign."""
    if not (a.is_finite() and b.is_finite()) or b.is_zero():
        return CTX.remainder_near(a, b) if near else CTX.remainder(a, b)
    ta, tb = a.as_tuple(), b.as_tuple()
    e = min(ta.exponent, tb.exponent)
    X = int("".join(map(str, ta.digits))) * 10 ** (ta.exponent - e) * (-1 if ta.sign else 1)
    Y = int("".join(map(str, tb.digits))) * 10 ** (tb.exponent - e) * (-1 if tb.sign else 1)
    q, rem = divmod(abs(X), abs(Y))  # truncated quotient of the magnitudes
    if near and (2 * rem > abs(Y) or (2 * rem == abs(Y) and q % 2 == 1)):
        rem -= abs(Y)
    r = rem if X >= 0 else -rem
    if len(str(abs(r))) > PREC:
        raise SystemExit(f"remainder of {a} and {b} does not fit")
    return D.Decimal((ta.sign if r == 0 else int(r < 0), tuple(int(ch) for ch in str(abs(r))), e))


def out(op, a="-", b="-", c="-", mode="-", want=""):
    s = lambda x: x if isinstance(x, str) else str(x)
    print("\t".join([op, s(a), s(b), s(c), s(mode), want]))


INT64 = 2 ** 63
for _ in range(N):
    a, b = pair()
    out("add", a, b, want=bfmt(CTX.add(a, b)))
    out("sub", a, b, want=bfmt(CTX.subtract(a, b)))
    out("mul", a, b, want=bfmt(CTX.multiply(a, b)))
    out("div", a, b, want=bfmt(CTX.divide(a, b)))
    for op in ("eq", "ne", "lt", "le", "gt", "ge"):
        out(op, a, b, want=str(cmp(op, a, b)).lower())
    for op in ("min", "max"):
        out(op, a, b, want=bfmt(minmax(op, a, b)))
    m = r.randrange(6)
    q = operand() if r.random() < 0.3 else mk(False, 1, r.randint(-10, 3))
    out("quantize", a, q, want=bfmt(a.quantize(q, rounding=D.ROUND_HALF_EVEN, context=CTX)))
    out("quantizemode", a, q, mode=m, want=bfmt(a.quantize(q, rounding=MODES[m], context=CTX)))
    for op, rd in (("trunc", D.ROUND_DOWN), ("floor", D.ROUND_FLOOR), ("ceil", D.ROUND_CEILING),
                   ("round", D.ROUND_HALF_UP), ("roundeven", D.ROUND_HALF_EVEN)):
        out(op, a, want=bfmt(a.to_integral_value(rounding=rd, context=CTX)))
    out("neg", a, want=bfmt(a.copy_negate()))
    if r.random() < 0.25:
        c = operand()
        out("fma", a, b, c, want=bfmt(CTX.fma(a, b, c)))
        out("sqrt", a, want=bfmt(CTX.sqrt(a)))
        out("rem", a, b, want=bfmt(exact_rem(a, b, True)))
        out("mod", a, b, want=bfmt(exact_rem(a, b, False)))
    if a.is_finite():
        t = a.to_integral_value(rounding=D.ROUND_DOWN)
        if -INT64 <= t < INT64:
            out("toint", a, want=str(int(t)))
        out("fmtf", a, want=ffmt(a))
