#!/usr/bin/env bash
# Fixture for the `deadcode` Makefile gate.
#
# The gate's REQUIRED RED cases are the two different ways it must fail, and the
# second one is why this fixture exists: the gate used to capture stdout only and
# branch on whether it was empty, so a run that analysed NOTHING printed
# "deadcode: ok" and passed. deadcode@v0.49.0 did exactly that against the Go
# 1.27 floor — stderr carried "packages contain errors", exit was 2, stdout was
# empty, and the gate reported success in 25 seconds.
#
# Each case stubs `deadcode` on PATH, so this is hermetic and does not analyse
# the real module.
#
# Run locally with `bash scripts/__tests__/deadcode-gate.test.sh`.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
pass=0
fail=0
ok()  { echo "✅ $1"; pass=$((pass + 1)); }
bad() { echo "❌ $1"; fail=$((fail + 1)); }

# stub <body> writes a fake `deadcode` and returns the dir to prepend to PATH.
stub() {
  local dir="$work/bin.$RANDOM"
  mkdir -p "$dir"
  printf '#!/usr/bin/env bash\n%s\n' "$1" > "$dir/deadcode"
  chmod +x "$dir/deadcode"
  echo "$dir"
}

# expect <want-rc> <label> <stub-body> [grep-for]
#
# want-rc is MAKE's exit code, not the recipe's. GNU make exits 2 for any failed
# recipe regardless of what the recipe returned, so 2 means "the gate failed" and
# the two failure modes are told apart by their message, which is what grep-for
# is for.
expect() {
  local want="$1" label="$2" body="$3" needle="${4:-}"
  local dir out got
  dir="$(stub "$body")"
  out="$(cd "$REPO" && PATH="$dir:$PATH" make deadcode 2>&1)"
  got=$?
  if [ "$got" != "$want" ]; then
    bad "$label (wanted rc=$want, got rc=$got)"
    return
  fi
  if [ -n "$needle" ] && ! printf '%s' "$out" | grep -q "$needle"; then
    bad "$label (rc correct, but output did not mention '$needle')"
    return
  fi
  ok "$label"
}

echo "--- pass: a clean analysis ---"
expect 0 "no findings and exit 0 -> pass" 'exit 0' "deadcode: ok"

echo "--- required red: dead code was found ---"
expect 2 "MUTATION findings on stdout -> fail" \
  'echo "func unused is unreachable"; exit 0' \
  "dead (unreachable) code found"

echo "--- required red: the analysis itself failed ---"
# The regression this fixture exists for. Empty stdout, message on stderr,
# non-zero exit — indistinguishable from "clean" to a gate that reads stdout only.
expect 2 "MUTATION exit 2 with stderr only -> fail, not a silent pass" \
  'echo "deadcode: packages contain errors" >&2; exit 2' \
  "could not analyse"
expect 2 "MUTATION the v0.49.0 signature exactly -> fail" \
  'echo "package requires newer Go version go1.27 (application built with go1.25)" >&2; exit 2' \
  "raise DEADCODE_VERSION"
# Non-zero with findings on stdout too: the analysis failure must win, because
# a partial result is not a result.
expect 2 "MUTATION exit 1 WITH stdout findings -> fail as an analysis error" \
  'echo "func x is unreachable"; echo "boom" >&2; exit 1' \
  "could not analyse"

echo "--- required red: the tool is missing entirely ---"
# System dirs only, so make and the shell still resolve but deadcode does not.
# A gate that cannot find its tool must say so rather than report a clean run.
dir_empty="$work/empty"
mkdir -p "$dir_empty"
out="$(cd "$REPO" && PATH="$dir_empty:/usr/bin:/bin" make deadcode 2>&1)"
rc=$?
if [ "$rc" = "2" ] && printf '%s' "$out" | grep -q "not on PATH"; then
  ok "deadcode absent from PATH -> fail"
else
  bad "deadcode absent from PATH did not fail cleanly (rc=$rc)"
fi

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
