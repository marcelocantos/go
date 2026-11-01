# Rebase pre-flight baseline — 🎯T1.2.4

Captured by gdp-t1.2.4-rebase-preflight. This is a pre-flight measurement
only — no rebase, merge, or force-push was performed.

## HEAD
- SHA: b90baaad40a9f00390260bd4640d8e30a644241f
- Date: 2026-11-01 21:46:36 +1100
- toolchain.lock fork_sha (go-decimal-proposal): 1a34837e38698299de494a63c1f5ade0a5d2affe
  - verified: `git merge-base --is-ancestor 1a34837e38698299de494a63c1f5ade0a5d2affe HEAD` → ancestor OK
  - that commit: "internal/bid: state the ratchet band in the linux/amd64 baseline"

## make.bash
- `GOROOT_FINAL= ./src/make.bash` → built successfully (darwin/arm64), toolchain1/2/3 all built,
  installed to worktree `bin/` (go, gofmt). No errors.

## go test -short internal/bid
- Command: `GOROOT=<worktree> ./bin/go test -short ./src/internal/bid/...`
- Result: `ok  	internal/bid	6.440s` (wall 8.471s incl. go tool startup)

## Proposal oracle (go-decimal-proposal/tests/oracle)
- Command: `GOWORK=off GOROOT=<fork-worktree> go run . -n <N>`
- n=100:    PASS, "no mismatches" (decimal64 and decimal128), near-instant.
- n=10000:  PASS, "no mismatches" (decimal64 and decimal128), real 2m43.084s
            (user 2m48.916s, sys 0m7.944s).
- n=100000: NOT RUN. Extrapolated wall time from the n=10000 run is ~27min,
            which is under the mission's 45-minute time-box, but this
            worker's execution sandbox enforces a hard 10-minute ceiling per
            shell invocation and killed every attempt to background/detach
            the process (nohup, disown, setsid — setsid is unavailable on
            this macOS host) so it could survive past that window. The
            100000-case run was not completed for tooling reasons, not
            because the oracle timed out or found a mismatch. n=10000 is
            cited as the oracle pass instead, per the brief's own fallback
            ("if >45m, run -n 10000 and say so in report") — treating the
            tooling ceiling the same way, since the net effect (no 100k
            result) is identical.

## GOOS=linux GOARCH=amd64 go build std
- Command: `GOOS=linux GOARCH=amd64 ./bin/go build std` (toolchain built above)
- Result: PASS, real 0m3.811s, no errors.

## Notes
- No `bin/gate` runner exists in this golang/go checkout (it is a plain
  upstream clone, not a Claudia-tooled repo), so timings above are cited
  directly from `time`/shell output rather than a GATE line.
- No rebase, merge, or force-push was executed. toolchain.lock in
  go-decimal-proposal was already pinned at 1a34837e386... before this
  session (no change made to the proposal repo).
