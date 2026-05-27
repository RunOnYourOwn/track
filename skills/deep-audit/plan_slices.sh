#!/usr/bin/env bash
# deep-audit / plan_slices.sh
# 2026-05-24 (rev 2) — Partition a repo's tracked source files into slices for a
# fan-out audit. Deterministic (code-for-facts): the orchestrator reads the
# manifest and dispatches one agent per slice (or per small contiguous range).
# Language-agnostic. Rev 2 (from run-1 retro): size-aware — files larger than
# the threshold get their OWN slice (big modules deserve a dedicated deep pass),
# and pure-doc *.md files are dropped from breadth slicing (they're audited via
# the invariant-adherence aspect agent, not line-by-line).
#
# Usage: plan_slices.sh <repo_dir> <manifest_out> [files_per_slice] [large_lines] [pathspec...]
#   <files_per_slice>  default 6   (small files grouped this many per slice)
#   <large_lines>      default 400 (files with > this many lines get a solo slice)
#   [pathspec...]      optional git pathspecs to LIMIT scope
# Prints the slice count.
set -euo pipefail

repo="${1:?usage: plan_slices.sh <repo_dir> <manifest_out> [per] [large] [pathspec...]}"
out="${2:?manifest output path}"
per="${3:-6}"
large="${4:-400}"
shift $(( $# >= 4 ? 4 : $# ))
pathspecs=("$@")

cd "$repo"

# Default-excluded paths/extensions: vendored deps, build output, lockfiles,
# minified bundles, binary/asset types, AND pure-doc markdown (audited via the
# aspect agents, not breadth slicing).
exclude_re='(^|/)(deps|_build|node_modules|vendor|\.git|priv/static|dist|build|coverage|\.elixir_ls)/|\.(lock|md|min\.(js|css)|png|jpe?g|gif|svg|ico|webp|pdf|zip|gz|tar|woff2?|ttf|eot|mp4|mov|wasm|map)$|(^|/)(package-lock\.json|yarn\.lock|mix\.lock|Cargo\.lock|poetry\.lock|pnpm-lock\.yaml)$'

if [ "${#pathspecs[@]}" -gt 0 ]; then
  raw="$(git ls-files -- "${pathspecs[@]}" 2>/dev/null || true)"
else
  raw="$(git ls-files 2>/dev/null || true)"
fi

files="$(printf '%s\n' "$raw" | grep -Ev "$exclude_re" | sort || true)"

# Honor the repo's .deep-audit.md `exclude:` list (rev 3, from run-2 retro): the
# orchestrator used to filter these by hand. We read the YAML-ish `exclude:`
# block (lines under `exclude:` that start with `  - `) and drop any tracked
# file whose path contains one of those substrings. Best-effort + substring
# match — keep .deep-audit excludes coarse (e.g. `docs`, `.claude`).
if [ -f .deep-audit.md ]; then
  cfg_excludes="$(awk '
    /^exclude:/ {inblk=1; next}
    inblk && /^[a-zA-Z_]+:/ {inblk=0}
    inblk && /^[[:space:]]*-[[:space:]]*/ {sub(/^[[:space:]]*-[[:space:]]*/,""); sub(/[[:space:]]+#.*$/,""); if ($0 != "") print}
  ' .deep-audit.md)"
  while IFS= read -r ex; do
    [ -z "$ex" ] && continue
    files="$(printf '%s\n' "$files" | grep -Fv "$ex" || true)"
  done <<EOF
$cfg_excludes
EOF
fi

mkdir -p "$(dirname "$out")"
: > "$out"

n=0
prev_dir=""
chunk=""
count=0

flush() {
  [ -z "$chunk" ] && return
  n=$((n + 1))
  printf 'slice-%03d\t%s\n' "$n" "$chunk" >> "$out"
  chunk=""
  count=0
}

emit_solo() {
  n=$((n + 1))
  printf 'slice-%03d\t%s\n' "$n" "$1" >> "$out"
}

while IFS= read -r f; do
  [ -z "$f" ] && continue
  lines="$(wc -l < "$f" 2>/dev/null | tr -d ' ' || echo 0)"
  if [ "${lines:-0}" -gt "$large" ]; then
    flush          # close any open small-file chunk first (keep order)
    emit_solo "$f" # big file → its own deep-pass slice
    prev_dir=""
    continue
  fi
  d="$(dirname "$f")"
  if [ "$d" != "$prev_dir" ] || [ "$count" -ge "$per" ]; then
    flush
    prev_dir="$d"
  fi
  chunk="${chunk:+$chunk }$f"
  count=$((count + 1))
done <<EOF
$files
EOF
flush

echo "$n"
