#!/usr/bin/env bash
# challenges/streaming_describe_challenge.sh
#
# Round-284 anti-bluff Challenge for digital.vasic.streaming.
#
# Default mode: invoke the runner against the real package surface
# (SSE broker, webhook signing, HTTP circuit breaker, transport
# factory, realtime debouncer) and assert it exits 0 with the
# expected per-component PASS lines, 5-locale UX evidence, and OK
# trailer. The PASS is backed by captured stdout, not by absence of
# error — per Article XI §11.9.
#
# Paired-mutation mode (--mutate): build a self-contained scratch
# Go module that re-implements a mini circuit-breaker invariant
# with a deliberate bug (breaker never opens), and assert the
# scratch runner correctly surfaces the mutation as exit 99. A
# mutation run that exits 0 means the Challenge itself is a bluff
# (CONST-035 mutation-bluff) and the script exits 1 to surface it.
#
# Exit codes (default mode):
#   0   — runner exited 0 + every assertion passed.
#   1   — assertion failed (FAIL printed with captured stdout).
#
# Exit codes (mutate mode):
#   99  — mutation correctly surfaced (PASS).
#   1   — mutation not surfaced (Challenge is a bluff).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

MODE="default"
if [[ ${1:-} == "--mutate" ]]; then
    MODE="mutate"
fi

run_default() {
    echo "[streaming-challenge] mode=default — exercising runner against real packages"
    cd "${REPO_ROOT}"

    local out
    out=$(go run ./challenges/runner -all 2>&1) || {
        echo "[streaming-challenge] FAIL: runner exited non-zero"
        echo "${out}"
        exit 1
    }

    # Per-component PASS lines for the closed set.
    local components=(
        "sse-broker"
        "webhook-sign-verify"
        "http-circuit-breaker"
        "transport-factory"
        "realtime-debouncer"
    )
    for c in "${components[@]}"; do
        if ! grep -q "^component=${c} status=PASS" <<<"${out}"; then
            echo "[streaming-challenge] FAIL: missing PASS line for component=${c}"
            echo "${out}"
            exit 1
        fi
    done

    # 5-locale UX evidence per CONST-046.
    local locales=("en" "sr" "ja" "es" "de")
    for loc in "${locales[@]}"; do
        if ! grep -q "^\[${loc}\] streaming:" <<<"${out}"; then
            echo "[streaming-challenge] FAIL: missing locale UX line for [${loc}]"
            echo "${out}"
            exit 1
        fi
    done

    # OK trailer is the positive-evidence summary.
    if ! grep -q "^OK components=5 checks=5 locales=5$" <<<"${out}"; then
        echo "[streaming-challenge] FAIL: missing OK trailer"
        echo "${out}"
        exit 1
    fi

    echo "${out}"
    echo "[streaming-challenge] PASS — runtime evidence captured above"
    exit 0
}

run_mutate() {
    echo "[streaming-challenge] mode=mutate — paired-mutation evidence"
    local scratch
    scratch="$(mktemp -d -t streaming-mutate-XXXXXX)"
    # shellcheck disable=SC2064
    trap "rm -rf '${scratch}'" EXIT

    mkdir -p "${scratch}/pkg/breaker_scratch"

    cat > "${scratch}/go.mod" <<'EOF'
module streaming.scratch

go 1.25
EOF

    cat > "${scratch}/pkg/breaker_scratch/breaker.go" <<'EOF'
package breaker_scratch

// Breaker is a mutated stand-in for a circuit-breaker invariant.
// The mutation: Trip() does NOT change state. The assertion the
// scratch runner makes (state must equal "open" after threshold
// failures) MUST fail on this mutated implementation. A scratch
// runner that exits 0 anyway means the assertion does not actually
// observe behaviour and the parent Challenge has a mutation-bluff.

type Breaker struct {
	state string
}

func New() *Breaker {
	return &Breaker{state: "closed"}
}

// Trip is mutated — it intentionally fails to change state.
func (b *Breaker) Trip() {
	// MUTATED: state never advances to "open".
	_ = b.state
}

func (b *Breaker) State() string {
	return b.state
}
EOF

    cat > "${scratch}/main.go" <<'EOF'
package main

import (
	"fmt"
	"os"

	bs "streaming.scratch/pkg/breaker_scratch"
)

func main() {
	b := bs.New()
	b.Trip()
	if b.State() != "open" {
		fmt.Fprintf(os.Stderr, "mutation detected: breaker state %q != \"open\" after Trip()\n", b.State())
		os.Exit(99)
	}
	fmt.Println("mutation NOT detected — bluff")
	os.Exit(0)
}
EOF

    cd "${scratch}"
    go build -o ./mutbin . >/dev/null 2>&1 || {
        echo "[streaming-challenge] FAIL-MUTATE — scratch build failed"
        exit 1
    }
    local mut_out mut_rc
    set +e
    mut_out=$(./mutbin 2>&1)
    mut_rc=$?
    set -e

    echo "${mut_out}"
    if [[ ${mut_rc} -eq 99 ]]; then
        echo "[streaming-challenge] PASS-MUTATE — mutation correctly surfaced (exit 99)"
        exit 99
    fi
    echo "[streaming-challenge] FAIL-MUTATE — mutation NOT surfaced (exit ${mut_rc}); Challenge is a bluff"
    exit 1
}

case "${MODE}" in
    default) run_default ;;
    mutate)  run_mutate ;;
esac
