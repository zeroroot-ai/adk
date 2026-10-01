#!/usr/bin/env bash
# check-readme-matches-cli.sh — the README documents the CLI the binary has.
#
# The README documented `gibson mission draft` and `gibson provider`, two
# command groups the cobra tree has no trace of, plus `--daemon`,
# `--insecure`, `GIBSON_DAEMON_ADDR` and `GIBSON_TENANT_ADDR`, none of which
# the binary reads. It also claimed a Go floor of 1.24 against a go.mod that
# says 1.26.8, so a person on 1.24 could not build. A newcomer following it
# got "unknown command" on their third step (adk#67).
#
# Two assertions, both keyed by content:
#
#   1. Every `gibson <command>` named in a README fenced block exists in the
#      cobra tree, read from `gibson docs cli --json` — the binary's own
#      machine-readable description of itself.
#   2. The Go floor the README states equals the `go` directive in
#      gibson/go.mod. One number, two files, so it cannot drift silently.
#
#   check-readme-matches-cli.sh             exit 1 on drift, 0 when clean
#   check-readme-matches-cli.sh --selftest  prove a fake command and a wrong floor both fail
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
README="${ROOT}/README.md"
GOMOD="${ROOT}/gibson/go.mod"

# commands_in_readme <file> — every "gibson <word> [<word>]" invocation inside
# a fenced block, as a space-joined path, deduplicated.
commands_in_readme() {
  awk '
    /^```/ { fenced = !fenced; next }
    !fenced { next }
    /^[[:space:]]*#/ { next }
    {
      sub(/#.*$/, "")                  # strip a trailing comment
      for (i = 1; i <= NF; i++) {
        if ($i != "gibson") continue
        path = ""
        for (j = i + 1; j <= NF; j++) {
          # a flag, placeholder or shell token ends the command path
          if ($j !~ /^[a-z][a-z0-9-]*$/) break
          path = (path == "" ? $j : path " " $j)
        }
        if (path != "") print path
      }
    }
  ' "$1" | sort -u
}

# commands_in_binary — every command path the cobra tree defines, from the
# binary's own JSON. Paths come back as "gibson mission submit"; the leading
# "gibson " is dropped so both sides compare in the same shape.
commands_in_binary() {
  # Every object in the document that carries a `path`, at any depth. The
  # root is {binary, commands, globalFlags, long, short} and nests
  # subcommands, so this reads the whole tree without assuming its depth.
  jq -r '.. | objects | select(has("path")) | .path' "$1" \
    | sed 's/^gibson //; /^gibson$/d' | sort -u
}

# parents_in_binary — the commands that HAVE subcommands. The distinction
# matters: after `gibson mission`, an unknown lowercase token is a command
# that does not exist, because `mission` is a group. After `gibson component
# init`, which is a leaf, the same-shaped token is a positional argument.
# Without this, `gibson component init prom-scanner` reads as drift and
# `gibson mission draft list` does not.
parents_in_binary() {
  jq -r '.. | objects | select(has("path") and ((.subcommands // []) | length > 0)) | .path' "$1" \
    | sed 's/^gibson //; /^gibson$/d' | sort -u
}

go_floor_readme() {
  grep -oP '(?<=<!-- go-floor -->)[^<]+' "$1" | head -1 || true
}

go_floor_gomod() {
  awk '$1 == "go" { print $2; exit }' "$GOMOD"
}

build_spec() {
  # `docs cli` writes the stable JSON document to stdout. There is no --json
  # flag; the command has one output shape.
  ( cd "$ROOT/gibson" && go run ./cmd/gibson docs cli ) >"$1"
}

# unknown_commands <known-file> <parents-file> <readme>
unknown_commands() {
  local known="$1" parents="$2" readme="$3"
  local cand prefix parent tok rest
  while IFS= read -r cand; do
    [ -n "$cand" ] || continue
    prefix=""
    rest="$cand"
    while [ -n "$rest" ]; do
      tok="${rest%% *}"
      [ "$tok" = "$rest" ] && rest="" || rest="${rest#* }"
      parent="$prefix"
      prefix="${prefix:+$prefix }$tok"
      if grep -qxF -- "$prefix" "$known"; then
        continue
      fi
      # $tok is not a command under $parent. Drift only if $parent is a
      # group (or the root), where a subcommand was the only thing it could
      # have been.
      if [ -z "$parent" ] || grep -qxF -- "$parent" "$parents"; then
        printf '%s\n' "$cand"
      fi
      break
    done
  done <"$readme"
}

check() {
  local spec="$1" readme="$2" rc=0
  local tmp known parents cands unknown
  tmp=$(mktemp -d)
  known="$tmp/known"; parents="$tmp/parents"; cands="$tmp/cands"
  commands_in_binary "$spec" >"$known"
  parents_in_binary "$spec" >"$parents"
  commands_in_readme "$readme" >"$cands"

  unknown=$(unknown_commands "$known" "$parents" "$cands")
  if [ -n "$unknown" ]; then
    echo "❌ the README names commands the binary does not define:"
    while IFS= read -r cmd; do
      [ -n "$cmd" ] && printf '     gibson %s\n' "$cmd"
    done <<<"$unknown"
    echo "   Delete them, or add them to the cobra tree. The binary is ground truth."
    rc=1
  fi

  local want have
  want=$(go_floor_gomod)
  have=$(go_floor_readme "$readme")
  if [ -z "$have" ]; then
    echo "❌ the README has no <!-- go-floor -->…<!-- /go-floor --> marker; the guard cannot check the Go floor"
    rc=1
  elif [ "$want" != "$have" ]; then
    echo "❌ the README states Go ${have}, gibson/go.mod says ${want}. One number, two files."
    rc=1
  fi

  rm -rf "$tmp"
  return $rc
}

selftest() {
  local tmp spec failures=0
  tmp=$(mktemp -d)
  # shellcheck disable=SC2064
  trap "rm -rf '$tmp'" EXIT
  spec="$tmp/cli.json"
  build_spec "$spec"

  # The real pair must pass, or the rest of the selftest proves nothing.
  if ! check "$spec" "$README" >/dev/null; then
    echo "::error::[selftest] the committed README does not match the binary"
    check "$spec" "$README" || true
    failures=$((failures + 1))
  fi

  # A command the tree does not define must fail. This is adk#67 itself.
  # shellcheck disable=SC2016  # the backticks are a literal Markdown fence
  { cat "$README"; printf '\n```\ngibson mission draft list\n```\n'; } >"$tmp/fake-cmd.md"
  if check "$spec" "$tmp/fake-cmd.md" >/dev/null 2>&1; then
    echo "::error::[selftest] a README naming 'gibson mission draft' was not caught"
    failures=$((failures + 1))
  fi

  # A wrong Go floor must fail.
  sed 's|<!-- go-floor -->[^<]*<!-- /go-floor -->|<!-- go-floor -->1.24<!-- /go-floor -->|' \
    "$README" >"$tmp/wrong-go.md"
  if check "$spec" "$tmp/wrong-go.md" >/dev/null 2>&1; then
    echo "::error::[selftest] a README stating the wrong Go floor was not caught"
    failures=$((failures + 1))
  fi

  # A missing marker must fail, so the guard cannot be silenced by deleting it.
  sed 's|<!-- go-floor -->[^<]*<!-- /go-floor -->|1.26.8|' "$README" >"$tmp/no-marker.md"
  if check "$spec" "$tmp/no-marker.md" >/dev/null 2>&1; then
    echo "::error::[selftest] a README with no go-floor marker was not caught"
    failures=$((failures + 1))
  fi

  # A real command must not be flagged.
  # shellcheck disable=SC2016  # the backticks are a literal Markdown fence
  { cat "$README"; printf '\n```\ngibson mission submit mission.cue\n```\n'; } >"$tmp/real-cmd.md"
  if ! check "$spec" "$tmp/real-cmd.md" >/dev/null; then
    echo "::error::[selftest] a real command was wrongly flagged"
    failures=$((failures + 1))
  fi

  if [ "$failures" -ne 0 ]; then
    echo "[selftest] $failures assertion(s) failed"
    return 1
  fi
  echo "SELFTEST PASS"
}

main() {
  local tmp spec
  tmp=$(mktemp -d)
  # shellcheck disable=SC2064
  trap "rm -rf '$tmp'" EXIT

  if [ "${1:-}" = "--selftest" ]; then
    selftest || return 1
  fi

  spec="$tmp/cli.json"
  build_spec "$spec"
  if ! check "$spec" "$README"; then
    return 1
  fi
  echo "✓ readme-matches-cli: $(commands_in_readme "$README" | wc -l) documented invocation(s) all resolve, Go floor $(go_floor_gomod) agrees with gibson/go.mod"
}

main "$@"
