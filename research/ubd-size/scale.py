# Code added per extra decimal128 `+`: (size(A4) - size(A1)) / 3, from
# scale/s.go, where A1, A2 and A4 contain one, two and four additions.
import re
def sizes(path):
    f = {}; cur = None
    for l in open(path):
        m = re.match(r'TEXT main\.(\S+)\(SB\)', l)
        if m: cur = m.group(1); f[cur] = [0, 0]; continue
        m = re.match(r'\s+\S+:\d+\s+0x[0-9a-f]+\s+([0-9a-f]+)\s+(.*)$', l)
        if m and cur and not m.group(2).strip().startswith('?'):
            f[cur][0] += 1; f[cur][1] += len(m.group(1)) // 2
    return f
print("\nper extra decimal128 +  (instr, bytes)")
for arch in ['amd64', 'arm64']:
    for b in ['base', 'ubd']:
        f = sizes(f'out/{b}-{arch}/scale.txt')
        a1, a4 = f['A1'], f['A4']
        print(f"{b:4} {arch}: {(a4[0]-a1[0])/3:.0f} instr, {(a4[1]-a1[1])/3:.0f} bytes")
