#!/bin/sh
# Record a reference trace of the original with openMSX + C-BIOS.
# usage: scripts/refdump.sh OUT.trace TICKS [KEYS] [ROM]
# Keep OUT outside the repository: traces are ROM-derived.
set -e
out=$1; ticks=${2:-600}; keys=${3:-}; romf=${4:-"Knightmare - Majyo Densetsu (1986) Konami [9832].rom"}
[ -n "$out" ] || { echo "usage: $0 OUT.trace TICKS [KEYS] [ROM]" >&2; exit 2; }
here=$(cd "$(dirname "$0")" && pwd)
RD_OUT=$out RD_TICKS=$ticks RD_KEYS=$keys \
  openmsx -machine C-BIOS_MSX1_JP -carta "$romf" -romtype Normal \
  -script "$here/refdump.tcl" >"$out.log" 2>&1 &
pid=$!
# Watchdog: about 1s per 60 ticks unthrottled is plenty; allow 30s + 1s/30 ticks.
limit=$((30 + ticks / 30))
i=0
while kill -0 $pid 2>/dev/null; do
  i=$((i + 1))
  if [ $i -gt $limit ]; then
    kill $pid; echo "refdump: watchdog killed openMSX after ${limit}s" >&2; exit 1
  fi
  sleep 1
done
wait $pid || true
if grep -qi 'error\|warning' "$out.log"; then cat "$out.log" >&2; fi
