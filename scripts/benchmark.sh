#!/bin/sh
set -eu
BIN=${1:-./bin/server-monitor}
DURATION=${DURATION:-300}
PORT=${PORT:-19090}
TOKEN=${SERVER_MONITOR_TOKEN:-benchmark-token}
SERVER_MONITOR_TOKEN="$TOKEN" SERVER_MONITOR_LISTEN="127.0.0.1:$PORT" "$BIN" >/tmp/server-monitor-bench.log 2>&1 &
pid=$!
trap 'kill $pid 2>/dev/null || true' EXIT INT TERM
sleep 2
start_ticks=$(awk '{print $14+$15}' /proc/$pid/stat)
start=$(date +%s)
peak=0
while kill -0 "$pid" 2>/dev/null && [ $(( $(date +%s)-start )) -lt "$DURATION" ]; do
  rss=$(awk '/VmRSS:/ {print $2}' /proc/$pid/status)
  [ "${rss:-0}" -gt "$peak" ] && peak=$rss
  sleep 1
done
end_ticks=$(awk '{print $14+$15}' /proc/$pid/stat)
elapsed=$(( $(date +%s)-start ))
hz=$(getconf CLK_TCK)
cpu=$(awk -v d=$((end_ticks-start_ticks)) -v hz="$hz" -v s="$elapsed" 'BEGIN { if(s>0) printf "%.3f", d/hz/s*100; else print "0" }')
printf 'duration_s=%s\npeak_rss_kb=%s\navg_cpu_pct=%s\n' "$elapsed" "$peak" "$cpu"
