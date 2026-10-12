#!/bin/sh
# Record a key schedule on the original and compare the port with it.
# usage: scripts/verify.sh KEYS OUTDIR [EXTRA_TICKS]
# The trace (ROM-derived) goes to OUTDIR, outside the repository.
set -e
keys=$1; dir=$2; extra=${3:-300}
[ -n "$keys" ] && [ -n "$dir" ] || { echo "usage: $0 KEYS OUTDIR [EXTRA_TICKS]" >&2; exit 2; }
here=$(cd "$(dirname "$0")" && pwd)
name=$(basename "$keys" .keys)
last=$(grep -v '^#' "$keys" | tail -1 | cut -d: -f1)
sh "$here/refdump.sh" "$dir/$name.trace" $((last + extra)) "$keys"
cd "$here/.."
go run ./cmd/refcompare -trace "$dir/$name.trace" -out "$dir/diff-$name"
go run ./cmd/tracecov -timeline "$dir/$name.trace" | awk 'NR > 1' | tail -4
