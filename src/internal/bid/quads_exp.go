package bid

// Experiment: a table of four digits an entry.
var quadTab [10000]uint32

func init() {
	for n := range quadTab {
		quadTab[n] = uint32('0'+n/1000) | uint32('0'+n/100%10)<<8 | uint32('0'+n/10%10)<<16 | uint32('0'+n%10)<<24
	}
}

func eightQuads(v uint64) uint64 {
	a := v * 3518437209 >> 45
	return uint64(quadTab[a]) | uint64(quadTab[v-a*10000])<<32
}
