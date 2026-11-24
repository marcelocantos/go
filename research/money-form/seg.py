#!/usr/bin/env python3
# Inline code per call site for the raw-word prototypes, by the method of
# ../ubd-size/seg.py: the wrapper's frame (stack check, prologue, epilogues,
# stack-growth stub) is excluded; what remains is the inline fast path plus
# the slow-call setup. Also reports the hot path: the instructions executed
# from entry to the fast result, taking at each conditional branch the side
# that does not reach the slow call.
#
# usage: seg.py out/amd64/objdump.txt out/arm64/objdump.txt
import re, sys

def load(path):
    funcs, cur = {}, None
    for l in open(path):
        m = re.match(r'TEXT moneyform/ops\.(\S+)\(SB\)', l)
        if m:
            cur = m.group(1); funcs[cur] = []; continue
        m = re.match(r'\s+\S+:\d+\s+0x([0-9a-f]+)\s+([0-9a-f]+)\s+(.*)$', l)
        if m and cur:
            asm = m.group(3).strip()
            if asm.startswith('?'): continue
            funcs[cur].append((int(m.group(1), 16), len(m.group(2)) // 2, asm))
    return funcs

PRO = [r'CMPQ SP, 0x10\(R14\)', r'JBE ', r'PUSHQ BP', r'MOVQ SP, BP', r'SUBQ \$0x[0-9a-f]+, SP',
       r'MOVD 16\(R28\), R16', r'CMP R16, RSP', r'BLS ', r'MOVD\.W R30', r'MOVD R29, -8\(RSP\)', r'SUB \$8, RSP, R29']
EPI = [r'LEAVE', r'RET', r'ADDQ \$0x[0-9a-f]+, SP', r'POPQ BP', r'MOVD -8\(RSP\), R29', r'MOVD\.P \d+\(RSP\), R30']

def target(a, s):
    m = re.search(r'(-?\d+)\(PC\)', s)
    if m: return a + 4 * int(m.group(1))
    m = re.search(r'\b0x([0-9a-f]+)\s*$', s)
    if m and re.match(r'(J|B|CB|TB)', s): return int(m.group(1), 16)
    return None

def is_branch(s):
    return re.match(r'(J[A-Z]+|B[A-Z]{2}|CBN?Z|TBN?Z|JMP|B) ', s) is not None

def analyse(ins):
    lastret = max(i for i, (_, _, s) in enumerate(ins) if s.startswith('RET'))
    body = ins[:lastret + 1]           # drop the stack-growth stub
    over, keep = [], []
    pro = True
    for i, (a, n, s) in enumerate(body):
        if pro and any(re.match(p, s) for p in PRO):
            over.append((a, n, s)); continue
        pro = False
        if any(re.match(p, s) for p in EPI):
            over.append((a, n, s)); continue
        keep.append((a, n, s))
    addr = {a: i for i, (a, _, _) in enumerate(body)}

    def slow_from(i):                  # does straight-line code from i reach a CALL before RET/JMP?
        while i < len(body):
            s = body[i][2]
            if 'CALL' in s: return True
            if s.startswith('RET') or is_branch(s): return False
            i += 1
        return False

    # hot path walk
    hot, i, seen = [], next(i for i, x in enumerate(body) if x in keep), set()
    while i < len(body) and i not in seen:
        seen.add(i)
        a, n, s = body[i]
        if s.startswith('RET'): break
        if 'CALL' in s: hot = None; break
        if body[i] in keep: hot.append((n, s))
        if is_branch(s):
            t = target(a, s)
            if s.startswith('JMP') or s.startswith('B '):
                i = addr[t]; continue
            if t in addr and not slow_from(addr[t]) and slow_from(i + 1):
                i = addr[t]; continue
        i += 1
    glue = []
    # slow-call setup: the block ending in each CALL
    for i, (a, n, s) in enumerate(body):
        if 'CALL' in s:
            j = i
            while j > 0 and not is_branch(body[j - 1][2]) and body[j - 1] in keep and not body[j-1][2].startswith('RET'):
                j -= 1
            glue += [(x[1], x[2]) for x in body[j:i + 1]]
    return over, keep, hot, glue

OPS = ['Add', 'Sub', 'Mul', 'Eq', 'Lt']
res = {}
for path in sys.argv[1:]:
    arch = 'amd64' if 'amd64' in path else 'arm64'
    for name, ins in load(path).items():
        if name[0] in 'UNC' and name[1:] in OPS:
            res[(arch, name)] = analyse(ins)

print('instructions/bytes: inline per call site (frame excluded) | hot path | slow-call setup')
for op in OPS:
    row = [f'{op:4}']
    for arch in ['amd64', 'arm64']:
        for v in 'UNC':
            over, keep, hot, glue = res[(arch, v + op)]
            row.append(f'{arch[:3]} {v} {len(keep):3}/{sum(n for _, n, _ in keep):3}B hot {len(hot):2} glue {len(glue)}')
    print(' | '.join(row))

if '-v' in sys.argv[0:1] or True:
    import os
    if os.environ.get('SHOW'):
        for k, (over, keep, hot, glue) in sorted(res.items()):
            if k[1] in os.environ['SHOW'].split(','):
                print('\n==', k)
                for n, s in hot: print('   ', s)
