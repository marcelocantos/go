package main

//go:noinline
func A1(a, b, c, d, e decimal128) decimal128 { return a + b }

//go:noinline
func A2(a, b, c, d, e decimal128) decimal128 { return a + b + c }

//go:noinline
func A4(a, b, c, d, e decimal128) decimal128 { return a + b + c + d + e }

func main() { println(A1(1, 2, 3, 4, 5), A2(1, 2, 3, 4, 5), A4(1, 2, 3, 4, 5)) }
