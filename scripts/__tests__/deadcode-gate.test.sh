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

# PINNED is read from the Makefile, not written here, so bumping the pin needs
# no edit in this file.
PINNED="$(awk -F':=' '/^DEADCODE_VERSION[[:space:]]*:=/{gsub(/[[:space:]]/,"",$2); print $2; exit}' "$REPO/Makefile")"
[ -n "$PINNED" ] || { echo "could not read DEADCODE_VERSION from the Makefile"; exit 1; }

# stub <body> [xtools-version] writes a fake `deadcode` plus a fake `go` whose
# `version -m` reports that x/tools version, and returns the dir to prepend to
# PATH. The gate asserts the binary it ran carries the pinned x/tools, so every
# case has to answer that probe; passing a different version is how the version
# cases are driven. An empty string makes the probe print nothing, which is what
# a non-Go binary on PATH looks like.
stub() {
  local dir="$work/bin.$RANDOM"
  local ver="${2-$PINNED}"
  local modname="${3:-golang.org/x/tools}"
  mkdir -p "$dir"
  printf '#!/usr/bin/env bash\n%s\n' "$1" > "$dir/deadcode"
  chmod +x "$dir/deadcode"
  {
    echo '#!/usr/bin/env bash'
    echo 'if [ "$1" = "version" ] && [ "$2" = "-m" ]; then'
    echo '  echo "$3: go1.27.1"'
    if [ -n "$ver" ]; then
      echo "  printf '\\tmod\\t${modname}\\t${ver}\\th1:stub=\\n'"
    fi
    echo '  exit 0'
    echo 'fi'
    # Any other `go` call from this recipe is unexpected, so fail loudly instead
    # of delegating to the real toolchain and hiding it.
    echo 'echo "fixture: the deadcode recipe called go $* — not expected" >&2'
    echo 'exit 97'
  } > "$dir/go"
  chmod +x "$dir/go"
  echo "$dir"
}

# expect <want-rc> <label> <stub-body> [grep-for]
#
# want-rc is MAKE's exit code, not the recipe's. GNU make exits 2 for any failed
# recipe regardless of what the recipe returned, so 2 means "the gate failed" and
# the two failure modes are told apart by their message, which is what grep-for
# is for.
expect() {
  local want="$1" label="$2" body="$3" needle="${4:-}" ver="${5-$PINNED}" modname="${6:-golang.org/x/tools}"
  local dir out got
  dir="$(stub "$body" "$ver" "$modname")"
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

echo "--- required red: PATH served a binary other than the pinned one ---"
# The defect this half exists for. `command -v deadcode` accepts whatever PATH
# resolves, so DEADCODE_VERSION was decorative: on one workstation PATH served an
# x/tools v0.44.0 deadcode out of an asdf Go 1.23.2 package dir. v0.44.0 exits 2
# on a Go 1.27 module, so the gate failed with the analysis-error message and told
# the reader to raise a pin that was already correct.
expect 2 "MUTATION an older x/tools on PATH -> fail naming the version" \
  'exit 0' "is x/tools v0.44.0, not ${PINNED}" "v0.44.0"
expect 2 "MUTATION a NEWER x/tools on PATH -> also fail; the pin is the pin" \
  'exit 0' "is x/tools v0.99.0, not ${PINNED}" "v0.99.0"
expect 2 "MUTATION a binary go cannot read -> fail, never assume it matches" \
  'exit 0' "unreadable" ""
# The probe must read the x/tools line specifically. A `mod` line for any other
# module carrying the pinned version must not satisfy it.
expect 2 "MUTATION the pinned version on a DIFFERENT module -> unreadable" \
  'exit 0' "unreadable" "$PINNED" "golang.org/x/mod"

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
