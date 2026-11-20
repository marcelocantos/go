#!/bin/bash
# Measure inline decimal128 code per call site: XOR-BID (baseline) vs UBD.
#
# usage: ./measure.sh BASE_GOROOT UBD_GOROOT
#   BASE_GOROOT  a toolchain built from 5032fd77 (Marcelo's decimal branch, XOR-BID)
#   UBD_GOROOT   a toolchain built from this branch (ubd128)
#
# UBD_GCFLAGS, if set, is passed as -gcflags to the UBD builds, for example
# UBD_GCFLAGS=-d=ubdcallonly=1 for the kernel call alone at each call site.
#
# Writes out/{base,ubd}-{amd64,arm64}/ and prints both tables.
set -euo pipefail
cd "$(dirname "$0")"
BASE=$1; UBD=$2
for v in base ubd; do
  GR=$BASE; GCF=; [ $v = ubd ] && GR=$UBD && GCF=${UBD_GCFLAGS:-}
  for a in amd64 arm64; do
    o=out/$v-$a; mkdir -p $o
    (cd probe && PATH=$GR/bin:$PATH GOTOOLCHAIN=local GOEXPERIMENT=decimal GOARCH=$a go build -gcflags="$GCF" -o ../$o/probe .)
    PATH=$GR/bin:$PATH go tool objdump -s '^main\.' $o/probe > $o/objdump.txt
    (cd scale && PATH=$GR/bin:$PATH GOTOOLCHAIN=local GOEXPERIMENT=decimal GOARCH=$a go build -gcflags="$GCF" -o ../$o/scale .)
    PATH=$GR/bin:$PATH go tool objdump -s '^main\.A' $o/scale > $o/scale.txt
  done
done
python3 seg.py
python3 scale.py
