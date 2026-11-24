# Splits each probe function into frame overhead, inline fast paths and
# slow-call setup, and prints per-op inline code (all fast paths + call setup,
# frame excluded). A fast path is a code region ending in a jump to the
# function epilogue (or RET); a region that contains a CALL, or that jumps
# elsewhere, is slow-call setup. Reads out/{base,ubd}-{amd64,arm64}/objdump.txt.

import re,sys
def load(path):
    funcs={};cur=None
    for l in open(path):
        m=re.match(r'TEXT main\.(\S+)\(SB\)',l)
        if m: cur=m.group(1); funcs[cur]=[]; continue
        m=re.match(r'\s+\S+:\d+\s+0x([0-9a-f]+)\s+([0-9a-f]+)\s+(.*)$',l)
        if m and cur:
            asm=m.group(3).strip()
            if asm.startswith('?'): continue
            funcs[cur].append((int(m.group(1),16),len(m.group(2))//2,asm))
    return funcs
def seg(ins,arch):
    # prologue: up to frame setup
    pe = next((i for i,(a,n,s) in enumerate(ins) if (arch=='amd64' and s.startswith('SUBQ $') and s.endswith('SP')) or (arch=='arm64' and re.match(r'SUB \$\d+, RSP, R29',s))),-1)+1
    ret = max(i for i,(a,n,s) in enumerate(ins) if s.startswith('RET'))
    addr={a:i for i,(a,n,s) in enumerate(ins)}
    def tgt(s,a=None):
        m=re.search(r'JMP 0x([0-9a-f]+)',s)
        if m: return int(m.group(1),16)
        m=re.search(r'JMP (-?\d+)\(PC\)',s)
        if m and a is not None: return a+4*int(m.group(1))
        return None
    # epilogue start: the earliest jump target that falls in the last block before RET
    targets=[tgt(s,a) for a,n,s in ins[pe:ret] if tgt(s,a)]
    near=[t for t in targets if t in addr and ret-8<=addr[t]<=ret]
    es = min(addr[t] for t in near) if near else ret
    if pe==0: es=len(ins)  # leaf: no frame, every RET is part of the path
    segs=[];cur=[];
    for i in range(pe,es):
        a,n,s=ins[i]; cur.append((n,s))
        if s.startswith('JMP') or s.startswith('RET'):
            t=tgt(s,a) if s.startswith('JMP') else None
            kind='fast' if (s.startswith('RET') or (t is not None and t in addr and addr[t]>=es)) and not any('CALL' in x for _,x in cur) else 'glue'
            if any('CALL' in x for _,x in cur): kind='glue'
            segs.append((kind,cur)); cur=[]
    if cur: segs.append(('fast' if not any('CALL' in x for _,x in cur) and not segs else 'glue',cur))
    over = ins[:pe]+ins[es:]
    return over,segs
def summ(f,arch,build):
    ins=load(f'out/{build}-{arch}/objdump.txt')
    return {k:seg(v,arch) for k,v in ins.items() if k in OPS}
OPS=['Add','Sub','Mul','Div','Eq','Lt','IsZero','Pos','Quantize','Round']
for arch in ['amd64','arm64']:
    print(f'\n== {arch}  (instr/bytes)   overhead | fast paths (each)                      | slow-call glue')
    for b in ['base','ubd']:
        r=summ(None,arch,b)
        for op in OPS:
            over,segs=r[op]
            fast=[(len(c),sum(n for n,_ in c)) for k,c in segs if k=='fast']
            glue=[(len(c),sum(n for n,_ in c)) for k,c in segs if k=='glue']
            g=(sum(x for x,_ in glue),sum(y for _,y in glue))
            print(f"{b:4} {op:9} {len(over):3}/{sum(n for _,n,_ in over):4} | {' '.join(f'{i}/{y}' for i,y in fast):38} | {g[0]}/{g[1]}")

print("\n\nop | arch | first fast path XOR-BID→UBD (instr, bytes) | all inline code excl. frame XOR-BID→UBD (instr, bytes)")
for arch in ['amd64','arm64']:
    rb=summ(None,arch,'base'); ru=summ(None,arch,'ubd')
    for op in OPS:
        def tot(r):
            over,segs=r[op]; f=[c for k,c in segs if k=='fast']
            first=(len(f[0]),sum(n for n,_ in f[0])) if f else (0,0)
            allc=(sum(len(c) for k,c in segs), sum(sum(n for n,_ in c) for k,c in segs))
            return first,allc
        (fb,ab),(fu,au)=tot(rb),tot(ru)
        pct=lambda a,b: f"{100*(b-a)/a:+.0f}%" if a else "n/a"
        print(f"{op:9}|{arch}| {fb[0]}→{fu[0]} ({pct(fb[0],fu[0])}), {fb[1]}→{fu[1]} ({pct(fb[1],fu[1])}) | {ab[0]}→{au[0]} ({pct(ab[0],au[0])}), {ab[1]}→{au[1]} ({pct(ab[1],au[1])})")
