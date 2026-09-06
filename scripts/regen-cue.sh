#!/usr/bin/env bash
# regen-cue.sh — regenerate the ADK's embedded mission CUE schema from
# the SDK proto.
#
# The ADK embeds a small CUE module at
#   gibson/cmd/gibson/cmd/mission/schema/
# so that `gibson mission validate` can resolve
#   import "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"
# without requiring the customer to have the SDK source tree on disk.
#
# The single file we regenerate is:
#   gibson/cmd/gibson/cmd/mission/schema/api/proto/gibson/mission/v1/
#     mission_definition_proto_gen.cue
#
# The other files in the schema bundle are hand-maintained and
# intentionally NOT regenerated — they exist exactly to wrap the
# auto-generated file in an offline-friendly module:
#   cue.mod/module.cue
#   api/proto/gibson/types/v1/types_stub.cue   (typespb)
#   api/proto/gibson/job/v1/job_stub.cue       (jobpb, sdk v0.177.0)
# A stub carries the minimum surface the mission schema needs, not the
# whole package. The fixture test TestCUESchemaValidation_JobNode guards a
# stub against proto drift: it runs a job-node mission through CUE and then
# through protojson into the SDK Go types.
#
# Pipeline
# --------
# 1. Run `cue import proto` against the SDK's mission_definition.proto.
#    This is the same invocation the SDK Makefile's `cue-defs` target uses,
#    but scoped to the single file we care about.
# 2. Apply two ADK-specific transforms:
#    (a) Rename the CUE package from `missionpb` to `v1`. The ADK schema
#        bundle uses positional `v1` so that import statements like
#        `import v1 "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"`
#        bind cleanly (CUE matches the last path segment if no alias is
#        given).
#    (b) Prepend the regen-pipeline sentinel header — the freshness gate
#        (scripts/check-cue-fresh.sh STRUCTURAL mode) greps for this
#        sentinel as proof the file went through the pipeline.
#    (c) Rewrite every `api/gen/...:<alias>` colon import into the
#        Go-style `<alias> "api/proto/..."` alias form, so it binds to
#        the `package v1` stub at the SDK-shaped path.
# 3. The raw `cue import proto` output imports sibling packages by their
#    Go path, `api/gen/gibson/<pkg>/v1`. The SDK rewrites that to
#    `api/proto/...` in its own tree (sdk#48), and so does transform (c)
#    here. The bundle therefore mirrors the SDK's api/proto tree: every
#    package a template can import lives at the same path in both.
#
# Usage
#   scripts/regen-cue.sh                  # write to in-tree path
#   scripts/regen-cue.sh /path/to/out.cue # write to a specific path
#                                         # (used by check-cue-fresh.sh)
#
# Prerequisites
#   - SDK sibling clone at ../sdk
#   - cue binary on PATH (cuelang.org/go/cmd/cue v0.16.x)
#   - buf protovalidate dependency cached at
#     $HOME/.cache/buf/.../protovalidate/.../files
#     (populated by `buf dep update` in the SDK repo)
#
# Spec
#   issue zeroroot-ai/adk#27 (CUE freshness gate, M3 of mission-author-
#   experience epic, parent PRD zeroroot-ai/gibson#131)

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ADK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SDK_ROOT="$(cd "$ADK_ROOT/../sdk" 2>/dev/null && pwd || true)"

DEFAULT_OUT="$ADK_ROOT/gibson/cmd/gibson/cmd/mission/schema/api/proto/gibson/mission/v1/mission_definition_proto_gen.cue"
OUT="${1:-$DEFAULT_OUT}"

if [[ -z "$SDK_ROOT" || ! -d "$SDK_ROOT/api/proto/gibson/mission/v1" ]]; then
  echo "[$SCRIPT_NAME] ERROR: SDK sibling not found at $ADK_ROOT/../sdk" >&2
  echo "  The regen pipeline reads $SDK_ROOT/api/proto/gibson/mission/v1/mission_definition.proto" >&2
  echo "  Clone the SDK as a sibling of the ADK and re-run." >&2
  exit 2
fi

if ! command -v cue >/dev/null 2>&1; then
  echo "[$SCRIPT_NAME] ERROR: cue binary not on PATH." >&2
  echo "  Install with: go install cuelang.org/go/cmd/cue@v0.16.1" >&2
  exit 2
fi

BUF_VALIDATE_DIR="$(find "$HOME/.cache/buf" -type d -path '*protovalidate*/files' 2>/dev/null | head -1)"
if [[ -z "$BUF_VALIDATE_DIR" ]]; then
  echo "[$SCRIPT_NAME] ERROR: protovalidate proto cache not found." >&2
  echo "  Run 'buf dep update' from the SDK directory first ($SDK_ROOT)." >&2
  exit 2
fi

WORK="$(mktemp -d -t adk-regen-cue.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT

# Copy the SDK proto tree into a workdir so `cue import` can resolve
# transitive imports without polluting the SDK's own tree with the
# generated *_proto_gen.cue files.
cp -r "$SDK_ROOT/api/proto" "$WORK/proto"

(
  cd "$WORK"
  # cue import proto writes *_proto_gen.cue alongside the .proto file
  # in the import roots. We point at the mission_definition.proto only.
  cue import proto -f --files \
    -I proto -I "$BUF_VALIDATE_DIR" \
    proto/gibson/mission/v1/mission_definition.proto
)

GENERATED="$WORK/proto/gibson/mission/v1/mission_definition_proto_gen.cue"
if [[ ! -f "$GENERATED" ]]; then
  echo "[$SCRIPT_NAME] ERROR: cue import produced no output at $GENERATED" >&2
  exit 2
fi

# --- ADK-specific transforms ---
#
# 1) Package rename: missionpb -> v1.
# 2) Prepend sentinel header right after the existing comment block
#    (before the `package` line). We use a stable marker so the
#    rewrite is idempotent.
#
# The transform is in python because sed/awk struggle with the multi-line
# block-edit + idempotency requirement.
python3 - "$GENERATED" "$OUT" <<'PYEOF'
import sys, re
src = open(sys.argv[1]).read()
out_path = sys.argv[2]

sentinel = """
// NOTE: This copy is embedded in the adk CLI binary for offline schema
// validation. It is regenerated from the SDK proto by
// scripts/regen-cue.sh — DO NOT EDIT BY HAND. The freshness gate
// (scripts/check-cue-fresh.sh) fails CI if this file drifts from the
// SDK proto. See zeroroot-ai/adk#27.
"""

# Remove any pre-existing sentinel block (idempotency).
src = re.sub(
    r"\n// NOTE: This copy is embedded in the adk CLI binary[^\n]*\n(// [^\n]*\n)*",
    "\n",
    src,
)

# Rewrite the package declaration.
src = re.sub(r"^package missionpb\b", "package v1", src, count=1, flags=re.M)

# Rewrite every api/gen import from CUE colon-alias form
#   "github.com/zeroroot-ai/sdk/api/gen/gibson/<pkg>/v1:<alias>"
# into the conventional Go-style alias form at the SDK-shaped path
#   <alias> "github.com/zeroroot-ai/sdk/api/proto/gibson/<pkg>/v1"
# Two things happen here. The path moves from api/gen to api/proto, the
# same rewrite the SDK applies in its own tree (sdk#48), so a template
# imports the job package at the path it has in the SDK. And the colon
# form becomes an alias: every stub in the embedded bundle declares
# `package v1` (matching its directory name), so the colon form (which
# requires the imported package to be literally named <alias>) fails to
# resolve, while the alias form binds `package v1` locally as <alias>.
#
# The rule is generic on purpose. mission_definition.proto imports typespb
# today and jobpb since sdk v0.177.0. A new api/gen import needs a matching
# stub directory under api/proto, not a new rewrite rule.
src = re.sub(
    r'"github\.com/zeroroot-ai/sdk/api/gen/([^":]+):(\w+)"',
    r'\2 "github.com/zeroroot-ai/sdk/api/proto/\1"',
    src,
)

# Inject the sentinel block immediately before the `package v1` line,
# keeping the leading newline so the sentinel is visually separated from
# the preceding doc comment.
src = re.sub(r"^(package v1\b)", sentinel + r"\1", src, count=1, flags=re.M)

import os
os.makedirs(os.path.dirname(out_path) or ".", exist_ok=True)
open(out_path, "w").write(src)
PYEOF

echo "[$SCRIPT_NAME] wrote $OUT"
