#!/usr/bin/env bash
#
# test_examples.sh — Run SDK examples and verify their output using VLLM.
#
# Discovers all runnable examples in examples/*, runs each one, then uses a
# small Go verifier backed by the VLLM SDK to evaluate whether the output
# matches expected behavior.
#
# Usage: scripts/test_examples.sh [options]
#   -n PARALLEL   Max examples to run concurrently (default: 5)
#   -t TIMEOUT    Per-example timeout in seconds (default: 120)
#   -o DIR        Output directory for logs (default: /tmp/sdk-example-tests-<timestamp>)
#   -s EXAMPLES   Comma-separated list of examples to skip
#   -f EXAMPLES   Comma-separated list of examples to run (filter, run only these)
#   -m MODEL      VLLM model to use for verification (default: VLLM_MODEL, then QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ)
#   -k            Keep going on failure (default: stop on first failure)
#   -h            Help
#

set -euo pipefail

# ---------------------------------------------------------------------------
# Portability helpers (macOS ships bash 3.2 which lacks mapfile, timeout, etc.)
# ---------------------------------------------------------------------------

# timeout(1) is not available on macOS by default (it's GNU coreutils).
# Provide a fallback that uses a background sleep + kill approach.
if ! command -v timeout >/dev/null 2>&1; then
    if command -v gtimeout >/dev/null 2>&1; then
        timeout() { gtimeout "$@"; }
    else
        timeout() {
            local secs="$1"; shift
            ("$@") &
            local cmd_pid=$!
            (sleep "$secs" && kill "$cmd_pid" 2>/dev/null) &
            local timer_pid=$!
            local rc=0
            wait "$cmd_pid" 2>/dev/null || rc=$?
            # If the timer is still running, the command finished on its own.
            if kill -0 "$timer_pid" 2>/dev/null; then
                kill "$timer_pid" 2>/dev/null || true
                wait "$timer_pid" 2>/dev/null || true
                return $rc
            fi
            # Timer already exited — it fired and killed the command.
            wait "$timer_pid" 2>/dev/null || true
            return 124
        }
    fi
fi

# ---------------------------------------------------------------------------
# Signal handling
# ---------------------------------------------------------------------------
INTERRUPTED=false
declare -a WORKER_PIDS=()

cleanup() {
    INTERRUPTED=true
    echo ""
    echo "Interrupted — killing workers..."
    if [[ ${#WORKER_PIDS[@]} -gt 0 ]]; then
        for pid in "${WORKER_PIDS[@]}"; do
            pkill -P "$pid" 2>/dev/null || true
            kill "$pid" 2>/dev/null || true
        done
        # Wait briefly then force-kill stragglers.
        sleep 0.3
        for pid in "${WORKER_PIDS[@]}"; do
            kill -9 "$pid" 2>/dev/null || true
        done
    fi
}

trap 'cleanup' INT TERM

# ---------------------------------------------------------------------------
# Defaults
# ---------------------------------------------------------------------------
PARALLEL=5
TIMEOUT=300
OUTDIR=""
SKIP_LIST=""
FILTER_LIST=""
VERIFY_MODEL=""
KEEP_GOING=false

# Directories that are not runnable examples.
SKIP_DIRS="internal"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
usage() {
    sed -n '/^# Usage:/,/^$/p' "$0" | sed 's/^# //'
    exit 0
}

die() { echo "ERROR: $*" >&2; exit 1; }

# Check if a value exists in a comma-separated list.
in_list() {
    local needle="$1" haystack="$2"
    [[ -z "$haystack" ]] && return 1
    IFS=',' read -ra items <<< "$haystack"
    for item in "${items[@]}"; do
        [[ "$item" == "$needle" ]] && return 0
    done
    return 1
}

# ---------------------------------------------------------------------------
# Parse flags
# ---------------------------------------------------------------------------
while getopts "n:t:o:s:f:m:kh" opt; do
    case "$opt" in
        n) PARALLEL="$OPTARG" ;;
        t) TIMEOUT="$OPTARG" ;;
        o) OUTDIR="$OPTARG" ;;
        s) SKIP_LIST="$OPTARG" ;;
        f) FILTER_LIST="$OPTARG" ;;
        m) VERIFY_MODEL="$OPTARG" ;;
        k) KEEP_GOING=true ;;
        h) usage ;;
        *) usage ;;
    esac
done

# ---------------------------------------------------------------------------
# Resolve paths
# ---------------------------------------------------------------------------
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EXAMPLES_DIR="$REPO_ROOT/examples"
[[ -d "$EXAMPLES_DIR" ]] || die "examples/ directory not found at $EXAMPLES_DIR"

if [[ -z "$OUTDIR" ]]; then
    OUTDIR="/tmp/sdk-example-tests-$(date +%Y%m%d-%H%M%S)"
fi
mkdir -p "$OUTDIR"

# Verify prerequisites.
command -v go >/dev/null 2>&1    || die "go not found in PATH"
command -v jq >/dev/null 2>&1    || die "jq not found in PATH"
[[ -n "${VLLM_API_KEY:-}" ]] || die "VLLM_API_KEY is required"

# ---------------------------------------------------------------------------
# Discover examples
# ---------------------------------------------------------------------------
examples=()
for dir in "$EXAMPLES_DIR"/*/; do
    name="$(basename "$dir")"

    # Skip non-runnable dirs.
    if in_list "$name" "$SKIP_DIRS"; then
        continue
    fi

    # Must contain main.go.
    [[ -f "$dir/main.go" ]] || continue

    # User-specified skip list.
    if in_list "$name" "$SKIP_LIST"; then
        continue
    fi

    # User-specified filter list.
    if [[ -n "$FILTER_LIST" ]] && ! in_list "$name" "$FILTER_LIST"; then
        continue
    fi

    examples+=("$name")
done

# Sort for deterministic order.
# mapfile requires bash 4+; use a while-read loop for bash 3.2 compatibility.
IFS=$'\n' read -r -d '' -a examples < <(printf '%s\n' "${examples[@]}" | sort; printf '\0') || true

total=${#examples[@]}
[[ $total -gt 0 ]] || die "No examples found to run"

echo "=== SDK Example Tests ==="
echo "Examples to run: $total"
echo "Parallel: $PARALLEL"
echo "Timeout per example: ${TIMEOUT}s"
echo "Output dir: $OUTDIR"
echo ""

# ---------------------------------------------------------------------------
# Per-example arguments and run commands
# ---------------------------------------------------------------------------
example_args() {
    local name="$1"
    case "$name" in
        cancellation|extended_thinking)
            echo "all"
            ;;
        *)
            echo ""
            ;;
    esac
}

run_example() {
    local name="$1"
    local logfile="$2"
    local run_rc=0

    case "$name" in
        custom_logger)
            # Has its own go.mod — run from within its directory.
            (cd "$EXAMPLES_DIR/custom_logger" && timeout "$TIMEOUT" go run .) \
                > "$logfile" 2>&1
            ;;
        *)
            local args
            args="$(example_args "$name")"
            if [[ -n "$args" ]]; then
                (cd "$REPO_ROOT" && timeout "$TIMEOUT" go run "./examples/$name" "$args") \
                    > "$logfile" 2>&1
            else
                (cd "$REPO_ROOT" && timeout "$TIMEOUT" go run "./examples/$name") \
                    > "$logfile" 2>&1
            fi
            ;;
    esac
}

# ---------------------------------------------------------------------------
# Parse a verifier JSON result file into verdict + reason.
# Writes two files: <name>.verdict ("true"/"false") and <name>.reason
# ---------------------------------------------------------------------------
parse_result() {
    local name="$1"
    local result_file="$OUTDIR/${name}.result.json"

    if [[ ! -s "$result_file" ]]; then
        echo "false" > "$OUTDIR/${name}.verdict"
        echo "verification failed (empty result)" > "$OUTDIR/${name}.reason"
        return
    fi

    local verdict reason
    verdict="$(jq -r '.pass // false' "$result_file" 2>/dev/null || echo "false")"
    reason="$(jq -r '.reason // "Unknown"' "$result_file" 2>/dev/null || echo "Unknown")"

    echo "$verdict" > "$OUTDIR/${name}.verdict"
    echo "$reason" > "$OUTDIR/${name}.reason"
}

# ---------------------------------------------------------------------------
# Worker: run one example end-to-end (run + verify), write results to files.
# Runs in a subshell via &.
# ---------------------------------------------------------------------------
process_example() {
    local name="$1"
    local logfile="$OUTDIR/${name}.log"
    local result_file="$OUTDIR/${name}.result.json"

    # --- Run ---------------------------------------------------------------
    local run_rc=0
    run_example "$name" "$logfile" || run_rc=$?

    # Check for panic on non-zero, non-timeout exit.
    if [[ $run_rc -ne 0 && $run_rc -ne 124 ]]; then
        if grep -q "^panic:" "$logfile" 2>/dev/null; then
            echo "false" > "$OUTDIR/${name}.verdict"
            echo "Runtime error (exit code $run_rc) — panic detected" > "$OUTDIR/${name}.reason"
            return
        fi
    fi

    # --- Verify with VLLM --------------------------------------------
    local verifier_cmd=(
        go run ./scripts/example_verifier
        -name "$name"
        -source "$EXAMPLES_DIR/$name/main.go"
        -log "$logfile"
    )
    if [[ -n "$VERIFY_MODEL" ]]; then
        verifier_cmd+=(-model "$VERIFY_MODEL")
    fi

    (
        cd "$REPO_ROOT"
        "${verifier_cmd[@]}"
    ) > "$result_file" 2>"$OUTDIR/${name}.verify.err" || true

    # --- Parse result ------------------------------------------------------
    parse_result "$name"
}

# ---------------------------------------------------------------------------
# Main loop — run workers with bounded parallelism
# ---------------------------------------------------------------------------

# Remove finished PIDs from the WORKER_PIDS array.
reap_workers() {
    local alive=()
    if [[ ${#WORKER_PIDS[@]} -gt 0 ]]; then
        for pid in "${WORKER_PIDS[@]}"; do
            if kill -0 "$pid" 2>/dev/null; then
                alive+=("$pid")
            fi
        done
    fi
    if [[ ${#alive[@]} -gt 0 ]]; then
        WORKER_PIDS=("${alive[@]}")
    else
        WORKER_PIDS=()
    fi
}

# Wait until worker count drops below PARALLEL.
wait_for_slot() {
    while true; do
        reap_workers
        [[ ${#WORKER_PIDS[@]} -lt $PARALLEL ]] && return
        # Brief sleep to avoid busy-waiting, then wait for any child.
        sleep 0.2
    done
}

for name in "${examples[@]}"; do
    [[ "$INTERRUPTED" == true ]] && break

    wait_for_slot
    [[ "$INTERRUPTED" == true ]] && break

    echo "  Starting $name..."
    process_example "$name" &
    WORKER_PIDS+=($!)
done

# Wait for all remaining workers.
if [[ ${#WORKER_PIDS[@]} -gt 0 ]]; then
    for pid in "${WORKER_PIDS[@]}"; do
        wait "$pid" 2>/dev/null || true
    done
fi

# ---------------------------------------------------------------------------
# Collect results and print summary
# ---------------------------------------------------------------------------
pass_count=0
fail_count=0

echo ""
echo "=== Example Test Results ==="
for name in "${examples[@]}"; do
    local_verdict="$(cat "$OUTDIR/${name}.verdict" 2>/dev/null || echo "false")"
    local_reason="$(cat "$OUTDIR/${name}.reason" 2>/dev/null || echo "No result")"

    if [[ "$local_verdict" == "true" ]]; then
        printf "  %-30s PASS  %s\n" "$name" "$local_reason"
        ((pass_count++)) || true
    else
        printf "  %-30s FAIL  %s\n" "$name" "$local_reason"
        ((fail_count++)) || true
        if [[ "$KEEP_GOING" != true ]]; then
            break
        fi
    fi
done

echo "=== $pass_count/$total passed, $fail_count failed ==="
echo ""
echo "Logs: $OUTDIR"

# Exit non-zero if any failures.
[[ $fail_count -eq 0 ]]
